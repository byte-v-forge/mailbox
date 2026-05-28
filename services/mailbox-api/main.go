package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/envx"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/grpcclient"
	"github.com/byte-v-forge/common-lib/grpchealth"
	"github.com/byte-v-forge/common-lib/hotstream"
	"github.com/byte-v-forge/common-lib/hotstreamnats"
	"github.com/byte-v-forge/common-lib/natseventbus"
	"github.com/byte-v-forge/common-lib/randx"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	browserautomationv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/browserautomation/v1"

	"mailboxapi/pb"
)

type config struct {
	listenAddr             string
	pgDSN                  string
	webhookHTTPAddr        string
	dashboardHTTPAddr      string
	dashboardStaticDir     string
	browserAutomationAddr  string
	coordinationRedisURL   string
	recentEmailRedisURL    string
	recentEmailCachePrefix string
	recentEmailCacheTTL    time.Duration
	recentEmailCacheMax    int
	platformNATSURL        string
	eventStreamName        string
	inboxLockPrefix        string
	inboxLockTTL           time.Duration
	inboxLockRetry         time.Duration
	providers              mailboxProviderRuntimeConfig
}

type server struct {
	pb.UnimplementedMailboxServiceServer

	emailBackend emailBackend
	operations   *operationStore
	activities   *mailboxActivities
	providers    mailboxProviderRuntimeConfig
	hot          *mailboxHotStream
	work         *mailboxWorkDispatcher
}

func main() {
	cfg := loadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	browserConn, err := newGRPCClient("browser automation", cfg.browserAutomationAddr)
	if err != nil {
		log.Fatalf("failed to connect browser automation: %v", err)
	}
	defer browserConn.Close()

	coordinationClient, closeCoordinationClient, err := newRequiredRedisClient(ctx, cfg.coordinationRedisURL, "MAILBOX_COORDINATION_REDIS_URL is required for mailbox coordination")
	if err != nil {
		log.Fatalf("failed to initialize mailbox coordination redis client: %v", err)
	}
	if closeCoordinationClient != nil {
		defer func() { _ = closeCoordinationClient() }()
	}
	recentEmailClient, closeRecentEmailClient, err := newRequiredRedisClient(ctx, cfg.recentEmailRedisURL, "MAILBOX_RECENT_EMAIL_REDIS_URL is required for mailbox recent email cache")
	if err != nil {
		log.Fatalf("failed to initialize mailbox recent email redis client: %v", err)
	}
	if closeRecentEmailClient != nil {
		defer func() { _ = closeRecentEmailClient() }()
	}

	recentCache := newRecentEmailCache(recentEmailClient, cfg.recentEmailCachePrefix, cfg.recentEmailCacheTTL, cfg.recentEmailCacheMax)
	mailboxStore, err := NewMailboxStore(ctx, cfg.pgDSN, recentCache)
	if err != nil {
		log.Fatalf("failed to initialize mailbox store: %v", err)
	}
	defer mailboxStore.Close()
	inboxLock := redisx.NewBestEffortLocker(coordinationClient, cfg.inboxLockPrefix, cfg.inboxLockTTL, cfg.inboxLockRetry)
	platformEventBus, closePlatformEventBus, err := newPlatformEventBus(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize platform event bus: %v", err)
	}
	if closePlatformEventBus != nil {
		defer closePlatformEventBus()
	}
	if platformEventBus == nil {
		log.Fatal("PLATFORM_NATS_URL is required for mailbox event workers")
	}
	hotBus, closeHotStream, err := newMailboxHotStreamBus(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to initialize mailbox hotstream: %v", err)
	}
	if closeHotStream != nil {
		defer closeHotStream()
	}
	hotEvents := newMailboxHotStream(hotBus)
	platformEmailEvents := newMailboxPlatformEvents(platformEventBus)
	mailWatcher := NewMailWatcher(mailboxStore, hotEvents)

	operations, err := newOperationStore(cfg.pgDSN)
	if err != nil {
		log.Fatalf("failed to initialize mailbox operation store: %v", err)
	}
	workDispatcher := newMailboxWorkDispatcher(operations.db, "mailbox-api")
	emailBackend := &EmailService{store: mailboxStore, watcher: mailWatcher, providers: cfg.providers, inboxLock: inboxLock, work: workDispatcher}

	pollConsumer, err := platformEventBus.PullWorkerConsumer(cfg.eventStreamName, eventcatalog.MailboxEmailPollRequested.Subject, eventcatalog.MailboxEmailPollRequested.ConsumerDurable, 10, 60*time.Second)
	if err != nil {
		log.Fatalf("failed to initialize mailbox email poll worker: %v", err)
	}

	fetchConsumer, err := platformEventBus.PullWorkerConsumer(cfg.eventStreamName, eventcatalog.MailboxInboxFetchRequested.Subject, eventcatalog.MailboxInboxFetchRequested.ConsumerDurable, 5, 5*time.Minute)
	if err != nil {
		log.Fatalf("failed to initialize mailbox inbox fetch worker: %v", err)
	}

	activities := newMailboxActivitiesForProviders(cfg.providers, browserautomationv1.NewBrowserAutomationServiceClient(browserConn), emailBackend, operations, hotEvents)

	registrationConsumer, err := platformEventBus.PullWorkerConsumer(cfg.eventStreamName, eventcatalog.MailboxRegistrationRequested.Subject, eventcatalog.MailboxRegistrationRequested.ConsumerDurable, 2, 5*time.Minute)
	if err != nil {
		log.Fatalf("failed to initialize mailbox registration worker: %v", err)
	}

	oauthConsumer, err := platformEventBus.PullWorkerConsumer(cfg.eventStreamName, eventcatalog.MailboxOAuthRequested.Subject, eventcatalog.MailboxOAuthRequested.ConsumerDurable, 2, 5*time.Minute)
	if err != nil {
		log.Fatalf("failed to initialize mailbox OAuth worker: %v", err)
	}

	errCh := make(chan error, 3)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return runMailboxPlatformEventOutboxWorker(groupCtx, mailboxStore, platformEmailEvents) })
	group.Go(func() error { return runMailboxEmailPollWorker(groupCtx, pollConsumer, emailBackend) })
	group.Go(func() error { return runMailboxInboxFetchWorker(groupCtx, fetchConsumer, emailBackend, operations) })
	group.Go(func() error {
		return runMailboxRegistrationWorker(groupCtx, registrationConsumer, operations, activities)
	})
	group.Go(func() error { return runMailboxOAuthWorker(groupCtx, oauthConsumer, operations, activities) })
	startWebhookServer(groupCtx, cfg.webhookHTTPAddr, mailboxStore, mailWatcher, inboxLock, errCh)

	listener, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", cfg.listenAddr, err)
	}

	mailboxServer := &server{
		emailBackend: emailBackend,
		operations:   operations,
		providers:    cfg.providers,
		hot:          hotEvents,
		work:         workDispatcher,
		activities:   activities,
	}
	grpcServer := grpc.NewServer()
	pb.RegisterMailboxServiceServer(grpcServer, mailboxServer)
	grpchealth.RegisterServing(grpcServer)

	dashboardConn, err := grpcclient.NewInsecure(selfTarget(cfg.listenAddr))
	if err != nil {
		log.Fatalf("connect mailbox dashboard API: %v", err)
	}
	defer dashboardConn.Close()
	startDashboardHTTP(groupCtx, cfg.dashboardHTTPAddr, cfg.dashboardStaticDir, pb.NewMailboxServiceClient(dashboardConn), hotBus, errCh)

	go func() {
		<-groupCtx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("mailbox API listening on %s", cfg.listenAddr)
	group.Go(func() error {
		if err := grpcServer.Serve(listener); err != nil {
			return fmt.Errorf("mailbox API failed: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		select {
		case <-groupCtx.Done():
			return nil
		case err := <-errCh:
			return err
		}
	})
	if err := group.Wait(); err != nil {
		stop()
		log.Fatal(err)
	}
}

func loadConfig() config {
	return config{
		listenAddr:             envx.StringDefault("LISTEN_ADDR", ":50051"),
		pgDSN:                  requiredEnv("MAILBOX_PG_DSN"),
		webhookHTTPAddr:        envx.StringDefault("MAILBOX_WEBHOOK_HTTP_ADDR", ":8082"),
		dashboardHTTPAddr:      envx.StringDefault("MAILBOX_DASHBOARD_HTTP_ADDR", ":8080"),
		dashboardStaticDir:     envx.StringDefault("MAILBOX_DASHBOARD_STATIC_DIR", "/app/dashboard/mailbox"),
		browserAutomationAddr:  envx.StringDefault("BROWSER_AUTOMATION_ADDR", "browser-automation:50051"),
		coordinationRedisURL:   envx.StringDefault("MAILBOX_COORDINATION_REDIS_URL", ""),
		recentEmailRedisURL:    envx.StringDefault("MAILBOX_RECENT_EMAIL_REDIS_URL", ""),
		recentEmailCachePrefix: envx.StringDefault("MAILBOX_RECENT_EMAIL_CACHE_KEY_PREFIX", "byte-v-forge:mailbox:recent-email"),
		recentEmailCacheTTL:    envx.PositiveDurationSeconds("MAILBOX_RECENT_EMAIL_CACHE_TTL_SECONDS", time.Hour),
		recentEmailCacheMax:    envx.PositiveInt("MAILBOX_RECENT_EMAIL_CACHE_MAX_MESSAGES", 20),
		platformNATSURL:        envx.StringDefault("PLATFORM_NATS_URL", ""),
		eventStreamName:        envx.StringDefault("PLATFORM_EVENT_STREAM_NAME", natseventbus.DefaultStream),
		inboxLockPrefix:        envx.StringDefault("MAILBOX_INBOX_LOCK_KEY_PREFIX", "byte-v-forge:mailbox:locks"),
		inboxLockTTL:           envx.PositiveDurationSeconds("MAILBOX_INBOX_LOCK_TTL_SECONDS", 10*time.Minute),
		inboxLockRetry:         envx.PositiveDurationSeconds("MAILBOX_INBOX_LOCK_RETRY_SECONDS", time.Second),
		providers:              loadMailboxProviderRuntimeConfig(),
	}
}

func newPlatformEventBus(ctx context.Context, cfg config) (*natseventbus.Bus, func(), error) {
	if strings.TrimSpace(cfg.platformNATSURL) == "" {
		return nil, nil, nil
	}
	bus, err := natseventbus.Connect(natseventbus.Config{
		URL:        cfg.platformNATSURL,
		ClientName: "mailbox-api",
	})
	if err != nil {
		return nil, nil, err
	}
	return bus, bus.Close, nil
}

func newMailboxHotStreamBus(ctx context.Context, cfg config) (hotstream.Bus, func(), error) {
	if strings.TrimSpace(cfg.platformNATSURL) == "" {
		return nil, nil, fmt.Errorf("PLATFORM_NATS_URL is required for mailbox hotstream")
	}
	bus, err := hotstreamnats.Connect(ctx, hotstreamnats.Config{
		URL:        cfg.platformNATSURL,
		ClientName: "mailbox-api",
		Subject:    hotstream.ServiceStateSubject("mailbox"),
	})
	if err != nil {
		return nil, nil, err
	}
	return bus, bus.Close, nil
}

func newRequiredRedisClient(ctx context.Context, redisURL string, requiredMessage string) (*redis.Client, func() error, error) {
	client, err := redisx.NewRequiredClient(ctx, redisURL, requiredMessage)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize redis client: %w", err)
	}
	return client, client.Close, nil
}

func requiredEnv(name string) string {
	if value := envx.String(name); value != "" {
		return value
	}
	log.Fatalf("%s is required", name)
	return ""
}

func newGRPCClient(name string, addr string) (*grpc.ClientConn, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("%s address is required", name)
	}
	return grpcclient.NewInsecure(addr)
}

type emailBackend interface {
	ListMailboxes(context.Context, *pb.ListEmailMailboxesRequest) (*pb.ListEmailMailboxesResponse, error)
	UpsertMailbox(context.Context, *pb.UpsertEmailMailboxRequest) (*pb.UpsertEmailMailboxResponse, error)
	DeleteMailbox(context.Context, *pb.DeleteMailboxRequest) (*pb.DeleteMailboxResponse, error)
	WaitForEmail(context.Context, *mailboxv1.WaitForMailboxEmailRequest) (*mailboxv1.WaitForMailboxEmailResponse, error)
	ListInbox(context.Context, *mailboxv1.ListMailboxInboxRequest) (*mailboxv1.ListMailboxInboxResponse, error)
	FetchInboxes(context.Context, *mailboxv1.FetchMailboxInboxesRequest) (*mailboxv1.FetchMailboxInboxesResponse, error)
	MarkEmailAuthStatus(context.Context, *pb.MarkEmailAuthStatusRequest) (*pb.MarkEmailAuthStatusResponse, error)
}

func (s *server) ListMailboxes(ctx context.Context, req *pb.ListEmailMailboxesRequest) (*pb.ListEmailMailboxesResponse, error) {
	resp, err := s.emailBackend.ListMailboxes(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "list mailboxes: %v", err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "email service returned empty mailbox list")
	}
	return resp, nil
}

func (s *server) UpsertMailbox(ctx context.Context, req *pb.UpsertEmailMailboxRequest) (*pb.UpsertEmailMailboxResponse, error) {
	mailbox := req.GetMailbox()
	if mailbox == nil || emailx.Normalize(mailbox.GetEmailAddress()) == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox email_address is required")
	}
	mailbox.EmailAddress = emailx.Normalize(mailbox.GetEmailAddress())
	if mailbox.GetProviderKey() == "" {
		mailbox.ProviderKey = defaultMailboxProvider()
	} else {
		mailbox.ProviderKey = normalizeMailboxProviderInput(mailbox.GetProviderKey())
	}
	resp, err := s.emailBackend.UpsertMailbox(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "upsert mailbox: %v", err)
	}
	if resp == nil || resp.GetMailbox() == nil {
		return nil, status.Error(codes.Internal, "email service returned empty mailbox")
	}
	return resp, nil
}

func (s *server) ListMailboxDomains(ctx context.Context, req *mailboxv1.ListMailboxDomainsRequest) (*mailboxv1.ListMailboxDomainsResponse, error) {
	return s.providers.ListDomains(req), nil
}

func (s *server) SyncMailboxDomains(ctx context.Context, req *mailboxv1.SyncMailboxDomainsRequest) (*mailboxv1.SyncMailboxDomainsResponse, error) {
	return s.providers.SyncDomains(req), nil
}

func (s *server) ListMailboxProviderCapabilities(ctx context.Context, req *mailboxv1.ListMailboxProviderCapabilitiesRequest) (*mailboxv1.ListMailboxProviderCapabilitiesResponse, error) {
	return s.providers.ListCapabilities(req), nil
}

func (s *server) DeleteMailbox(ctx context.Context, req *pb.DeleteMailboxRequest) (*pb.DeleteMailboxResponse, error) {
	email := emailx.Normalize(req.GetEmailAddress())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email_address is required")
	}
	resp, err := s.emailBackend.DeleteMailbox(ctx, &pb.DeleteMailboxRequest{EmailAddress: email})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "delete mailbox: %v", err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "email service returned empty delete response")
	}
	return resp, nil
}

func (s *server) WaitForMailboxEmail(ctx context.Context, req *mailboxv1.WaitForMailboxEmailRequest) (*mailboxv1.WaitForMailboxEmailResponse, error) {
	email := emailx.Normalize(req.GetEmailAddress())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email_address is required")
	}
	resp, err := s.emailBackend.WaitForEmail(ctx, &mailboxv1.WaitForMailboxEmailRequest{
		EmailAddress:    email,
		SubjectKeyword:  strings.TrimSpace(req.GetSubjectKeyword()),
		TimeoutSeconds:  req.GetTimeoutSeconds(),
		IssuedAfterUnix: req.GetIssuedAfterUnix(),
		ParserProfile:   req.GetParserProfile(),
		SignalKind:      req.GetSignalKind(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "wait for mailbox email: %v", err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "email service returned empty wait response")
	}
	return resp, nil
}

func (s *server) RegisterMailbox(ctx context.Context, req *mailboxv1.RegisterMailboxRequest) (*mailboxv1.RegisterMailboxResponse, error) {
	operationID := operationID("mailbox-register")
	if _, err := s.operations.createRegistration(ctx, operationID, req.GetImportOnly()); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	if s.work == nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_registration", ErrorMessage: "mailbox event dispatcher is not configured"})
		return nil, status.Error(codes.Unavailable, "mailbox event dispatcher is not configured")
	}
	if err := s.work.PublishRegistrationRequested(ctx, operationID); err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_registration", ErrorMessage: err.Error()})
		return nil, status.Errorf(codes.Unavailable, "queue mailbox registration: %v", err)
	}

	return &mailboxv1.RegisterMailboxResponse{
		OperationId: operationID,
		Started:     true,
	}, nil
}

func (s *server) RunMailboxOAuth(ctx context.Context, req *mailboxv1.StartMailboxOAuthRequest) (*mailboxv1.StartMailboxOAuthResponse, error) {
	operationID := operationID("mailbox-oauth")
	email := emailx.Normalize(req.GetEmailAddress())
	if _, err := s.operations.createOAuth(ctx, operationID, email, req.GetOnlyMissing(), req.GetLimit()); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	if s.work == nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_oauth", ErrorMessage: "mailbox event dispatcher is not configured"})
		return nil, status.Error(codes.Unavailable, "mailbox event dispatcher is not configured")
	}
	if err := s.work.PublishOAuthRequested(ctx, operationID); err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_oauth", ErrorMessage: err.Error()})
		return nil, status.Errorf(codes.Unavailable, "queue mailbox OAuth: %v", err)
	}

	return &mailboxv1.StartMailboxOAuthResponse{
		OperationId: operationID,
		Started:     true,
	}, nil
}

func (s *server) ListMailboxInbox(ctx context.Context, req *mailboxv1.ListMailboxInboxRequest) (*mailboxv1.ListMailboxInboxResponse, error) {
	resp, err := s.emailBackend.ListInbox(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "list mailbox inbox: %v", err)
	}
	if resp == nil || resp.GetResult() == nil {
		return nil, status.Error(codes.Internal, "email service returned empty inbox result")
	}
	return resp, nil
}

func (s *server) FetchMailboxInboxes(ctx context.Context, req *mailboxv1.FetchMailboxInboxesRequest) (*mailboxv1.FetchMailboxInboxesResponse, error) {
	operationID := operationID("mailbox-inbox")
	email := emailx.Normalize(req.GetEmailAddress())
	if _, err := s.operations.create(ctx, operationID, operationActionFetchInboxes, email); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	request := &mailboxv1.FetchMailboxInboxesRequest{
		LimitPerMailbox:   req.GetLimitPerMailbox(),
		MaxMailboxes:      req.GetMaxMailboxes(),
		EmailAddress:      email,
		ParserProfile:     req.GetParserProfile(),
		ReceivedAfterUnix: req.GetReceivedAfterUnix(),
	}
	if s.work == nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_fetch_inboxes", ErrorMessage: "mailbox event dispatcher is not configured"})
		return nil, status.Error(codes.Unavailable, "mailbox event dispatcher is not configured")
	}
	if err := s.work.PublishInboxFetchRequested(ctx, operationID, request); err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "queue_fetch_inboxes", ErrorMessage: err.Error()})
		return nil, status.Errorf(codes.Unavailable, "queue mailbox inbox fetch: %v", err)
	}
	return &mailboxv1.FetchMailboxInboxesResponse{OperationId: operationID}, nil
}

func (s *server) GetMailboxOperation(ctx context.Context, req *mailboxv1.GetMailboxOperationRequest) (*mailboxv1.GetMailboxOperationResponse, error) {
	operationID := strings.TrimSpace(req.GetOperationId())
	if operationID == "" {
		return &mailboxv1.GetMailboxOperationResponse{ErrorMessage: "operation_id is required"}, nil
	}
	operation, err := s.operations.get(ctx, operationID)
	if err != nil {
		return &mailboxv1.GetMailboxOperationResponse{ErrorMessage: err.Error()}, nil
	}
	return &mailboxv1.GetMailboxOperationResponse{Operation: operation}, nil
}

func (s *server) ListMailboxOperations(ctx context.Context, req *mailboxv1.ListMailboxOperationsRequest) (*mailboxv1.ListMailboxOperationsResponse, error) {
	operations, err := s.operations.list(ctx, operationListFilter{
		Limit:        int(req.GetLimit()),
		Status:       req.GetStatus(),
		Action:       req.GetAction(),
		EmailAddress: req.GetEmailAddress(),
	})
	if err != nil {
		return &mailboxv1.ListMailboxOperationsResponse{ErrorMessage: err.Error()}, nil
	}
	return &mailboxv1.ListMailboxOperationsResponse{Operations: operations}, nil
}

func (s *server) updateOperation(ctx context.Context, operationID string, update operationUpdate) {
	operation, err := s.operations.update(ctx, operationID, update)
	if err != nil {
		log.Printf("update mailbox operation failed operation=%s: %v", operationID, err)
		return
	}
	s.hot.PublishOperation(ctx, operation)
}

func normalizedLimit(limit int32) int32 {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func operationID(prefix string) string {
	if id, err := randx.Hex(8); err == nil {
		return prefix + "-" + id
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
