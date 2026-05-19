package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"mailboxapi/pb"
)

type config struct {
	listenAddr          string
	pgDSN               string
	emailAddr           string
	mailboxRegisterAddr string
}

type server struct {
	pb.UnimplementedMailboxServiceServer

	emailClient           pb.EmailServiceClient
	mailboxRegisterClient pb.MailboxRegistrationServiceClient
	operations            *operationStore
}

func main() {
	cfg := loadConfig()
	emailConn, err := newGRPCClient("email service", cfg.emailAddr)
	if err != nil {
		log.Fatalf("failed to connect email service: %v", err)
	}
	defer emailConn.Close()

	registerConn, err := newGRPCClient("mailbox registration service", cfg.mailboxRegisterAddr)
	if err != nil {
		log.Fatalf("failed to connect mailbox registration service: %v", err)
	}
	defer registerConn.Close()

	operations, err := newOperationStore(cfg.pgDSN)
	if err != nil {
		log.Fatalf("failed to initialize mailbox operation store: %v", err)
	}

	listener, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", cfg.listenAddr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMailboxServiceServer(grpcServer, &server{
		emailClient:           pb.NewEmailServiceClient(emailConn),
		mailboxRegisterClient: pb.NewMailboxRegistrationServiceClient(registerConn),
		operations:            operations,
	})

	log.Printf("mailbox API listening on %s", cfg.listenAddr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("mailbox API failed: %v", err)
	}
}

func loadConfig() config {
	return config{
		listenAddr:          envDefault("LISTEN_ADDR", ":50051"),
		pgDSN:               requiredEnvAny("MAILBOX_API_PG_DSN", "PG_DSN"),
		emailAddr:           envDefault("EMAIL_ADDR", "outlook-imap-service:50051"),
		mailboxRegisterAddr: envDefault("MAILBOX_REGISTER_ADDR", "outlook-register-service:50051"),
	}
}

func envDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func requiredEnvAny(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	log.Fatalf("%s is required", strings.Join(names, " or "))
	return ""
}

func newGRPCClient(name string, addr string) (*grpc.ClientConn, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("%s address is required", name)
	}
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func (s *server) ListMailboxes(ctx context.Context, req *pb.ListEmailMailboxesRequest) (*pb.ListEmailMailboxesResponse, error) {
	resp, err := s.emailClient.ListMailboxes(ctx, req)
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
	if mailbox == nil || normalizeEmail(mailbox.GetEmailAddress()) == "" {
		return nil, status.Error(codes.InvalidArgument, "mailbox email_address is required")
	}
	mailbox.EmailAddress = normalizeEmail(mailbox.GetEmailAddress())
	if mailbox.GetPrimaryEmail() == "" {
		mailbox.PrimaryEmail = mailbox.GetEmailAddress()
	}
	resp, err := s.emailClient.UpsertMailbox(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "upsert mailbox: %v", err)
	}
	if resp == nil || resp.GetMailbox() == nil {
		return nil, status.Error(codes.Internal, "email service returned empty mailbox")
	}
	return resp, nil
}

func (s *server) DeleteMailbox(ctx context.Context, req *pb.DeleteMailboxRequest) (*pb.DeleteMailboxResponse, error) {
	email := normalizeEmail(req.GetEmailAddress())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email_address is required")
	}
	resp, err := s.emailClient.DeleteMailbox(ctx, &pb.DeleteMailboxRequest{EmailAddress: email})
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "delete mailbox: %v", err)
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "email service returned empty delete response")
	}
	return resp, nil
}

func (s *server) RegisterMailbox(ctx context.Context, req *pb.RegisterMailboxRequest) (*pb.RegisterMailboxResponse, error) {
	operationID := operationID("mailbox-register")
	if _, err := s.operations.create(ctx, operationID, operationActionRegisterMailbox, ""); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	if _, err := s.operations.update(ctx, operationID, operationUpdate{
		Status:   operationStatusRunning,
		LastStep: "run_registration",
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "mark mailbox operation running: %v", err)
	}

	resp, err := s.mailboxRegisterClient.RunMailboxRegistration(ctx, &pb.RunMailboxRegistrationRequest{
		Enabled:    !req.GetImportOnly(),
		ImportOnly: req.GetImportOnly(),
	})
	if err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "run_registration",
			ErrorMessage: err.Error(),
		})
		return nil, status.Errorf(codes.Unavailable, "run mailbox registration: %v", err)
	}
	if resp == nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "run_registration",
			ErrorMessage: "mailbox registration service returned empty response",
		})
		return nil, status.Error(codes.Internal, "mailbox registration service returned empty response")
	}

	out := &pb.RegisterMailboxResponse{
		OperationId:  operationID,
		Success:      resp.GetSuccess(),
		ExitCode:     resp.GetExitCode(),
		ErrorMessage: resp.GetErrorMessage(),
		Mailboxes:    make([]*pb.RegisteredMailbox, 0, len(resp.GetAccounts())),
	}
	for _, account := range resp.GetAccounts() {
		email := normalizeEmail(account.GetEmailAddress())
		if email == "" {
			continue
		}
		out.Mailboxes = append(out.Mailboxes, &pb.RegisteredMailbox{
			EmailAddress: email,
			Password:     strings.TrimSpace(account.GetPassword()),
			RefreshToken: strings.TrimSpace(account.GetRefreshToken()),
			AccessToken:  strings.TrimSpace(account.GetAccessToken()),
			Status:       mailboxStatus(account),
		})
	}
	if !out.GetSuccess() && out.GetErrorMessage() == "" {
		out.ErrorMessage = "mailbox registration failed"
	}
	statusValue := operationStatusSucceeded
	if !out.GetSuccess() || strings.TrimSpace(out.GetErrorMessage()) != "" {
		statusValue = operationStatusFailed
	}
	s.updateOperation(ctx, operationID, operationUpdate{
		Status:       statusValue,
		LastStep:     "run_registration",
		ErrorMessage: out.GetErrorMessage(),
		ExitCode:     out.GetExitCode(),
		MailboxCount: int32(len(out.GetMailboxes())),
	})
	return out, nil
}

func (s *server) RunMailboxOAuth(ctx context.Context, req *pb.StartMailboxOAuthRequest) (*pb.StartMailboxOAuthResponse, error) {
	operationID := operationID("mailbox-oauth")
	email := normalizeEmail(req.GetEmailAddress())
	if _, err := s.operations.create(ctx, operationID, operationActionMailboxOAuth, email); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	if _, err := s.operations.update(ctx, operationID, operationUpdate{
		Status:   operationStatusRunning,
		LastStep: "run_oauth",
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "mark mailbox operation running: %v", err)
	}

	resp, err := s.mailboxRegisterClient.RunMailboxOAuth(ctx, &pb.RunMailboxOAuthRequest{
		EmailAddress: email,
		OnlyMissing:  req.GetOnlyMissing(),
		Limit:        normalizedLimit(req.GetLimit()),
	})
	if err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "run_oauth",
			ErrorMessage: err.Error(),
		})
		return &pb.StartMailboxOAuthResponse{OperationId: operationID, ErrorMessage: err.Error()}, nil
	}
	if resp == nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "run_oauth",
			ErrorMessage: "mailbox registration service returned empty OAuth response",
		})
		return &pb.StartMailboxOAuthResponse{OperationId: operationID, ErrorMessage: "mailbox registration service returned empty OAuth response"}, nil
	}
	statusValue := operationStatusSucceeded
	if !resp.GetSuccess() || strings.TrimSpace(resp.GetErrorMessage()) != "" {
		statusValue = operationStatusFailed
	}
	s.updateOperation(ctx, operationID, operationUpdate{
		Status:       statusValue,
		LastStep:     "run_oauth",
		ErrorMessage: resp.GetErrorMessage(),
		MailboxCount: resp.GetProcessed(),
		FetchedCount: resp.GetSucceeded(),
		FailedCount:  resp.GetFailed(),
	})
	return &pb.StartMailboxOAuthResponse{
		OperationId:  operationID,
		Started:      resp.GetSuccess(),
		ErrorMessage: resp.GetErrorMessage(),
	}, nil
}

func (s *server) FetchMailboxInboxes(ctx context.Context, req *pb.FetchMailboxInboxesRequest) (*pb.FetchMailboxInboxesResponse, error) {
	operationID := operationID("mailbox-inbox")
	email := normalizeEmail(req.GetEmailAddress())
	if _, err := s.operations.create(ctx, operationID, operationActionFetchInboxes, email); err != nil {
		return nil, status.Errorf(codes.Internal, "create mailbox operation: %v", err)
	}
	if _, err := s.operations.update(ctx, operationID, operationUpdate{
		Status:   operationStatusRunning,
		LastStep: "fetch_inboxes",
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "mark mailbox operation running: %v", err)
	}

	resp, err := s.emailClient.FetchInboxes(ctx, &pb.FetchInboxesRequest{
		LimitPerMailbox: req.GetLimitPerMailbox(),
		MaxMailboxes:    req.GetMaxMailboxes(),
		EmailAddress:    email,
	})
	if err != nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "fetch_inboxes",
			ErrorMessage: err.Error(),
		})
		return nil, status.Errorf(codes.Unavailable, "fetch mailbox inboxes: %v", err)
	}
	if resp == nil {
		s.updateOperation(ctx, operationID, operationUpdate{
			Status:       operationStatusFailed,
			LastStep:     "fetch_inboxes",
			ErrorMessage: "email service returned empty inbox response",
		})
		return nil, status.Error(codes.Internal, "email service returned empty inbox response")
	}
	statusValue := operationStatusSucceeded
	if resp.GetFailedCount() > 0 {
		statusValue = operationStatusFailed
	}
	s.updateOperation(ctx, operationID, operationUpdate{
		Status:       statusValue,
		LastStep:     "fetch_inboxes",
		MailboxCount: resp.GetMailboxCount(),
		FetchedCount: resp.GetFetchedCount(),
		FailedCount:  resp.GetFailedCount(),
		MessageCount: resp.GetMessageCount(),
	})
	return &pb.FetchMailboxInboxesResponse{
		Results:      resp.GetResults(),
		MailboxCount: resp.GetMailboxCount(),
		FetchedCount: resp.GetFetchedCount(),
		FailedCount:  resp.GetFailedCount(),
		MessageCount: resp.GetMessageCount(),
		OperationId:  operationID,
	}, nil
}

func (s *server) GetMailboxOperation(ctx context.Context, req *pb.GetMailboxOperationRequest) (*pb.GetMailboxOperationResponse, error) {
	operationID := strings.TrimSpace(req.GetOperationId())
	if operationID == "" {
		return &pb.GetMailboxOperationResponse{ErrorMessage: "operation_id is required"}, nil
	}
	operation, err := s.operations.get(ctx, operationID)
	if err != nil {
		return &pb.GetMailboxOperationResponse{ErrorMessage: err.Error()}, nil
	}
	return &pb.GetMailboxOperationResponse{Operation: operation}, nil
}

func (s *server) ListMailboxOperations(ctx context.Context, req *pb.ListMailboxOperationsRequest) (*pb.ListMailboxOperationsResponse, error) {
	operations, err := s.operations.list(ctx, operationListFilter{
		Limit:        int(req.GetLimit()),
		Status:       req.GetStatus(),
		Action:       req.GetAction(),
		EmailAddress: req.GetEmailAddress(),
	})
	if err != nil {
		return &pb.ListMailboxOperationsResponse{ErrorMessage: err.Error()}, nil
	}
	return &pb.ListMailboxOperationsResponse{Operations: operations}, nil
}

func (s *server) updateOperation(ctx context.Context, operationID string, update operationUpdate) {
	if _, err := s.operations.update(ctx, operationID, update); err != nil {
		log.Printf("update mailbox operation failed operation=%s: %v", operationID, err)
	}
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
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

func mailboxStatus(account *pb.MailboxRegistrationAccount) string {
	if account == nil {
		return "OAUTH_PENDING"
	}
	if strings.TrimSpace(account.GetRefreshToken()) != "" {
		return "AUTHORIZED"
	}
	return "OAUTH_PENDING"
}

func operationID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(bytes[:])
	}
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}
