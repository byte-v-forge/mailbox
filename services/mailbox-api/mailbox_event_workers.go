package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventcatalog"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	commonv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/common/v1"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"mailboxapi/pb"
)

const defaultMailboxWorkRetryDelay = 5 * time.Second

type mailboxWorkDispatcher struct {
	db     *gorm.DB
	source string
}

func newMailboxWorkDispatcher(db *gorm.DB, source string) *mailboxWorkDispatcher {
	if db == nil {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = mailboxPlatformEventSource
	}
	return &mailboxWorkDispatcher{db: db, source: source}
}

func (d *mailboxWorkDispatcher) PublishRegistrationRequested(ctx context.Context, operationID string) error {
	return d.publishOperationRequested(ctx, eventcatalog.MailboxRegistrationRequested, "mailbox-registration-", operationID, &pb.MailboxRegistrationOperationRequest{OperationId: strings.TrimSpace(operationID)})
}

func (d *mailboxWorkDispatcher) PublishOAuthRequested(ctx context.Context, operationID string) error {
	return d.publishOperationRequested(ctx, eventcatalog.MailboxOAuthRequested, "mailbox-oauth-", operationID, &pb.MailboxOAuthOperationRequest{OperationId: strings.TrimSpace(operationID)})
}

func (d *mailboxWorkDispatcher) publishOperationRequested(ctx context.Context, definition eventcatalog.Definition, eventPrefix string, operationID string, request proto.Message) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("mailbox work dispatcher is not configured")
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	eventCtx := d.context(definition.EventName, eventbus.StableEventID(eventPrefix, operationID), operationID)
	return d.enqueue(ctx, eventbus.Message{
		Subject:    definition.Subject,
		Event:      request,
		Context:    eventCtx,
		Attributes: eventbus.Attributes("operation_id", operationID),
	})
}

func (d *mailboxWorkDispatcher) PublishEmailPollRequested(ctx context.Context, request *mailboxv1.MailboxEmailPollRequest) error {
	if d == nil || d.db == nil || request == nil {
		return nil
	}
	request.EmailAddress = emailx.Normalize(request.GetEmailAddress())
	eventCtx := d.context(
		eventcatalog.MailboxEmailPollRequested.EventName,
		eventbus.StableEventID("mailbox-email-poll-", request.GetEmailAddress(), request.GetSubjectKeyword(), request.GetParserProfile(), request.GetSignalKind().String(), fmt.Sprintf("%d", request.GetIssuedAfterUnix()), fmt.Sprintf("%d", request.GetDeadlineUnix())),
		request.GetEmailAddress(),
	)
	return d.enqueue(ctx, eventbus.Message{
		Subject: eventcatalog.MailboxEmailPollRequested.Subject,
		Event:   request,
		Context: eventCtx,
		Attributes: eventbus.Attributes(
			"email_address", request.GetEmailAddress(),
			"signal_kind", request.GetSignalKind().String(),
			"reason", request.GetReason(),
		),
	})
}

func (d *mailboxWorkDispatcher) PublishInboxFetchRequested(ctx context.Context, operationID string, request *mailboxv1.FetchMailboxInboxesRequest) error {
	if d == nil || d.db == nil {
		return fmt.Errorf("mailbox work dispatcher is not configured")
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if request == nil {
		request = &mailboxv1.FetchMailboxInboxesRequest{}
	}
	eventCtx := d.context(eventcatalog.MailboxInboxFetchRequested.EventName, eventbus.StableEventID("mailbox-inbox-fetch-", operationID), operationID)
	return d.enqueue(ctx, eventbus.Message{
		Subject: eventcatalog.MailboxInboxFetchRequested.Subject,
		Event: &pb.MailboxInboxFetchRequest{
			OperationId: operationID,
			Request:     proto.Clone(request).(*mailboxv1.FetchMailboxInboxesRequest),
		},
		Context: eventCtx,
		Attributes: eventbus.Attributes(
			"operation_id", operationID,
			"email_address", emailx.Normalize(request.GetEmailAddress()),
		),
	})
}

func (d *mailboxWorkDispatcher) context(eventName string, eventID string, correlationID string) *commonv1.EventContext {
	return eventbus.NewEventContext(eventbus.EventContextConfig{
		EventID:       eventID,
		EventName:     eventName,
		EventVersion:  mailboxPlatformEventVersion,
		SourceService: d.source,
		CorrelationID: correlationID,
	})
}

func (d *mailboxWorkDispatcher) enqueue(ctx context.Context, message eventbus.Message) error {
	record, err := eventoutbox.NewRecord(message)
	if err != nil {
		return err
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return eventoutbox.InsertRecordGORM(ctx, tx, mailboxPlatformEventOutboxTable, record, time.Now().Unix())
	})
}

type mailboxEmailPollWorker struct {
	service *EmailService
}

func runMailboxEmailPollWorker(ctx context.Context, consumer eventbus.Consumer, service *EmailService) error {
	worker := &mailboxEmailPollWorker{service: service}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "mailbox email poll requests",
		Consumer: consumer,
		Handler:  worker.handle,
	})
}

func (w *mailboxEmailPollWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	request, ok := decodeMailboxEmailPollRequest(message)
	if !ok {
		eventbus.TermMessage(ctx, message, "terminate malformed mailbox email poll request", nil)
		return
	}
	email := emailx.Normalize(request.GetEmailAddress())
	if email == "" {
		eventbus.TermMessage(ctx, message, "terminate mailbox email poll request without email", nil)
		return
	}
	if w.service.providers.IsStoredInboxOnlyAddress(email) {
		eventbus.AckMessage(ctx, message, "ack stored-inbox-only mailbox email poll request", nil)
		return
	}
	if deadlineReached(request.GetDeadlineUnix()) {
		eventbus.AckMessage(ctx, message, "ack expired mailbox email poll request", nil)
		return
	}
	if err := w.service.watcher.PollForEmail(ctx, email); err != nil {
		if isAuthError(err) {
			log.Printf("mailbox email poll auth failure email=%s: %v", emailx.Redact(email), err)
			eventbus.TermMessage(ctx, message, "terminate auth-failed mailbox email poll request", nil)
			return
		}
		delay := mailboxPollRetryDelay(err, w.service.watcher.pollInterval)
		log.Printf("mailbox email poll failed email=%s: %v", emailx.Redact(email), err)
		eventbus.NakMessageDelay(ctx, message, delay, "delay mailbox email poll retry", nil)
		return
	}
	if _, found, err := w.service.latestEmailResponse(ctx, &mailboxv1.WaitForMailboxEmailRequest{
		EmailAddress:    email,
		SubjectKeyword:  request.GetSubjectKeyword(),
		ParserProfile:   request.GetParserProfile(),
		SignalKind:      request.GetSignalKind(),
		IssuedAfterUnix: request.GetIssuedAfterUnix(),
	}, request.GetIssuedAfterUnix()); err != nil {
		log.Printf("mailbox email poll projection check failed email=%s: %v", emailx.Redact(email), err)
		eventbus.NakMessageDelay(ctx, message, defaultMailboxWorkRetryDelay, "retry mailbox email poll projection", nil)
		return
	} else if found {
		eventbus.AckMessage(ctx, message, "ack found mailbox email poll request", nil)
		return
	}
	if deadlineReached(request.GetDeadlineUnix()) {
		eventbus.AckMessage(ctx, message, "ack timed-out mailbox email poll request", nil)
		return
	}
	eventbus.NakMessageDelay(ctx, message, mailboxPollInterval(w.service.watcher.pollInterval), "delay mailbox email poll", nil)
}

type mailboxInboxFetchWorker struct {
	service    *EmailService
	operations *operationStore
}

func runMailboxInboxFetchWorker(ctx context.Context, consumer eventbus.Consumer, service *EmailService, operations *operationStore) error {
	worker := &mailboxInboxFetchWorker{service: service, operations: operations}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "mailbox inbox fetch requests",
		Consumer: consumer,
		Handler:  worker.handle,
	})
}

func (w *mailboxInboxFetchWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	request, ok := decodeMailboxInboxFetchRequest(message)
	if !ok {
		eventbus.TermMessage(ctx, message, "terminate malformed mailbox inbox fetch request", nil)
		return
	}
	operationID := strings.TrimSpace(request.GetOperationId())
	if operationID == "" {
		eventbus.TermMessage(ctx, message, "terminate mailbox inbox fetch request without operation_id", nil)
		return
	}
	if operation, err := w.operations.get(ctx, operationID); err == nil {
		if operation.GetStatus() == operationStatusSucceeded || operation.GetStatus() == operationStatusFailed {
			eventbus.AckMessage(ctx, message, "ack finalized mailbox inbox fetch request", nil)
			return
		}
	}
	_, _ = w.operations.update(ctx, operationID, operationUpdate{Status: operationStatusRunning, LastStep: "fetch_inboxes"})
	resp, err := w.service.FetchInboxes(ctx, request.GetRequest())
	if err != nil {
		_, _ = w.operations.update(ctx, operationID, operationUpdate{Status: operationStatusFailed, LastStep: "fetch_inboxes", ErrorMessage: err.Error()})
		eventbus.AckMessage(ctx, message, "ack failed mailbox inbox fetch request", nil)
		return
	}
	statusValue := operationStatusSucceeded
	if resp.GetFailedCount() > 0 {
		statusValue = operationStatusFailed
	}
	_, _ = w.operations.update(ctx, operationID, operationUpdate{
		Status:       statusValue,
		LastStep:     "fetch_inboxes",
		MailboxCount: resp.GetMailboxCount(),
		FetchedCount: resp.GetFetchedCount(),
		FailedCount:  resp.GetFailedCount(),
		MessageCount: resp.GetMessageCount(),
	})
	eventbus.AckMessage(ctx, message, "ack mailbox inbox fetch request", nil)
}

type mailboxRegistrationWorker struct {
	operations *operationStore
	activities *mailboxActivities
}

func runMailboxRegistrationWorker(ctx context.Context, consumer eventbus.Consumer, operations *operationStore, activities *mailboxActivities) error {
	worker := &mailboxRegistrationWorker{operations: operations, activities: activities}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "mailbox registration requests",
		Consumer: consumer,
		Handler:  worker.handle,
	})
}

func (w *mailboxRegistrationWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	request, ok := decodeMailboxRegistrationRequest(message)
	if !ok {
		eventbus.TermMessage(ctx, message, "terminate malformed mailbox registration request", nil)
		return
	}
	operationID := strings.TrimSpace(request.GetOperationId())
	start, err := w.operations.startRegistrationWorkerRun(ctx, operationID)
	if err != nil {
		handleOperationStartError(ctx, message, operationID, "mailbox registration", err)
		return
	}
	if start.Final {
		eventbus.AckMessage(ctx, message, "ack finalized mailbox registration request", nil)
		return
	}
	result, err := w.activities.runMailboxRegistration(ctx, mailboxRegistrationActionInput{OperationID: operationID, ImportOnly: start.ImportOnly})
	if err != nil {
		log.Printf("mailbox registration operation failed operation_id=%s success=%t: %v", operationID, result.Success, err)
	}
	eventbus.AckMessage(ctx, message, "ack mailbox registration request", nil)
}

type mailboxOAuthWorker struct {
	operations *operationStore
	activities *mailboxActivities
}

func runMailboxOAuthWorker(ctx context.Context, consumer eventbus.Consumer, operations *operationStore, activities *mailboxActivities) error {
	worker := &mailboxOAuthWorker{operations: operations, activities: activities}
	return eventbus.RunConsumerWorker(ctx, eventbus.ConsumerWorkerConfig{
		Name:     "mailbox OAuth requests",
		Consumer: consumer,
		Handler:  worker.handle,
	})
}

func (w *mailboxOAuthWorker) handle(ctx context.Context, message eventbus.ReceivedMessage) {
	request, ok := decodeMailboxOAuthRequest(message)
	if !ok {
		eventbus.TermMessage(ctx, message, "terminate malformed mailbox OAuth request", nil)
		return
	}
	operationID := strings.TrimSpace(request.GetOperationId())
	start, err := w.operations.startOAuthWorkerRun(ctx, operationID)
	if err != nil {
		handleOperationStartError(ctx, message, operationID, "mailbox OAuth", err)
		return
	}
	if start.Final {
		eventbus.AckMessage(ctx, message, "ack finalized mailbox OAuth request", nil)
		return
	}
	result := w.activities.runMailboxOAuthAction(ctx, operationID, start.EmailAddress, start.OnlyMissing, normalizedLimit(start.Limit))
	if !result.Success {
		log.Printf("mailbox OAuth operation failed operation_id=%s error=%s", operationID, result.ErrorMessage)
	}
	eventbus.AckMessage(ctx, message, "ack mailbox OAuth request", nil)
}

func handleOperationStartError(ctx context.Context, message eventbus.ReceivedMessage, operationID string, label string, err error) {
	if errors.Is(err, errOperationAlreadyRunning) {
		eventbus.NakMessageDelay(ctx, message, 30*time.Second, "delay busy "+label+" request", nil)
		return
	}
	if errors.Is(err, errOperationInvalidAction) || errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("%s request is invalid operation_id=%s: %v", label, operationID, err)
		eventbus.TermMessage(ctx, message, "terminate invalid "+label+" request", nil)
		return
	}
	log.Printf("start %s request failed operation_id=%s: %v", label, operationID, err)
	eventbus.NakMessageDelay(ctx, message, defaultMailboxWorkRetryDelay, "retry "+label+" request", nil)
}

func decodeMailboxRegistrationRequest(message eventbus.ReceivedMessage) (*pb.MailboxRegistrationOperationRequest, bool) {
	request := &pb.MailboxRegistrationOperationRequest{}
	if err := eventbus.UnmarshalPayload(message, request); err != nil {
		log.Printf("decode mailbox registration request failed event_id=%s: %v", eventbus.EventID(message), err)
		return nil, false
	}
	return request, strings.TrimSpace(request.GetOperationId()) != ""
}

func decodeMailboxOAuthRequest(message eventbus.ReceivedMessage) (*pb.MailboxOAuthOperationRequest, bool) {
	request := &pb.MailboxOAuthOperationRequest{}
	if err := eventbus.UnmarshalPayload(message, request); err != nil {
		log.Printf("decode mailbox OAuth request failed event_id=%s: %v", eventbus.EventID(message), err)
		return nil, false
	}
	return request, strings.TrimSpace(request.GetOperationId()) != ""
}

func decodeMailboxEmailPollRequest(message eventbus.ReceivedMessage) (*mailboxv1.MailboxEmailPollRequest, bool) {
	request := &mailboxv1.MailboxEmailPollRequest{}
	if err := eventbus.UnmarshalPayload(message, request); err != nil {
		log.Printf("decode mailbox email poll request failed event_id=%s: %v", eventbus.EventID(message), err)
		return nil, false
	}
	return request, true
}

func decodeMailboxInboxFetchRequest(message eventbus.ReceivedMessage) (*pb.MailboxInboxFetchRequest, bool) {
	request := &pb.MailboxInboxFetchRequest{}
	if err := eventbus.UnmarshalPayload(message, request); err != nil {
		log.Printf("decode mailbox inbox fetch request failed event_id=%s: %v", eventbus.EventID(message), err)
		return nil, false
	}
	return request, true
}

func mailboxPollRetryDelay(err error, configuredInterval int) time.Duration {
	var graphErr *GraphFetchError
	if errors.As(err, &graphErr) && graphErr.RetryAfter > 0 {
		return graphErr.RetryAfter
	}
	return mailboxPollInterval(configuredInterval)
}

func mailboxPollInterval(configuredInterval int) time.Duration {
	if configuredInterval <= 0 {
		return defaultMailboxWorkRetryDelay
	}
	return time.Duration(configuredInterval) * time.Second
}

func deadlineReached(deadlineUnix int64) bool {
	return deadlineUnix > 0 && !time.Now().Before(time.Unix(deadlineUnix, 0))
}
