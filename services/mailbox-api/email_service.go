package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/redisx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mailboxapi/pb"
)

type EmailService struct {
	store     *MailboxStore
	watcher   *MailWatcher
	providers mailboxProviderRuntimeConfig
	inboxLock *redisx.BestEffortLocker
	work      *mailboxWorkDispatcher
}

func (s *EmailService) acquireInboxLock(ctx context.Context) (func(), error) {
	if s.inboxLock == nil {
		return nil, status.Error(codes.Unavailable, "mailbox inbox lock is not configured")
	}
	lock, err := s.inboxLock.Lock(ctx, "fetch-inboxes")
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, status.Error(codes.DeadlineExceeded, "inbox fetch wait timeout")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, status.Error(codes.Canceled, "request cancelled")
		}
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := lock.Unlock(unlockCtx); err != nil {
			logWarning("release mailbox inbox lock failed: %v", err)
		}
	}, nil
}

func (s *EmailService) MarkEmailAuthStatus(ctx context.Context, request *pb.MarkEmailAuthStatusRequest) (*pb.MarkEmailAuthStatusResponse, error) {
	mailbox, err := s.store.MarkEmailAuthStatus(ctx, request.GetEmailAddress(), request.GetAuthStatus(), request.GetLastError())
	if err != nil {
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	}
	return &pb.MarkEmailAuthStatusResponse{Mailbox: mailbox}, nil
}

func (s *EmailService) UpsertMailbox(ctx context.Context, request *pb.UpsertEmailMailboxRequest) (*pb.UpsertEmailMailboxResponse, error) {
	mailbox, err := s.store.UpsertMailbox(ctx, request.GetMailbox())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pb.UpsertEmailMailboxResponse{Mailbox: mailbox}, nil
}

func (s *EmailService) ListMailboxes(ctx context.Context, request *pb.ListEmailMailboxesRequest) (*pb.ListEmailMailboxesResponse, error) {
	mailboxes, err := s.store.ListMailboxes(ctx, request.GetAuthStatus(), request.GetProviderKey(), request.GetLimit())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.ListEmailMailboxesResponse{Mailboxes: mailboxes}, nil
}

func (s *EmailService) DeleteMailbox(ctx context.Context, request *pb.DeleteMailboxRequest) (*pb.DeleteMailboxResponse, error) {
	deleted, err := s.store.DeleteMailbox(ctx, request.GetEmailAddress())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &pb.DeleteMailboxResponse{Deleted: deleted}, nil
}

func (s *EmailService) FetchInboxes(ctx context.Context, request *mailboxv1.FetchMailboxInboxesRequest) (*mailboxv1.FetchMailboxInboxesResponse, error) {
	unlock, err := s.acquireInboxLock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	type inboxTarget struct {
		fetchMailbox  *pb.EmailMailbox
		resultMailbox *pb.EmailMailbox
	}
	targets := []inboxTarget{}
	requestedEmail := emailx.Normalize(request.GetEmailAddress())
	if requestedEmail != "" {
		if resultMailbox, ok := s.providers.StoredInboxOnlyMailbox(requestedEmail); ok {
			if mailbox, err := s.store.FindMailbox(ctx, requestedEmail); err == nil {
				resultMailbox = mailbox
			}
			messages, err := s.store.ListInboxMessagesSince(ctx, requestedEmail, request.GetLimitPerMailbox(), request.GetReceivedAfterUnix())
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			return &mailboxv1.FetchMailboxInboxesResponse{
				MailboxCount: int32(1),
				FetchedCount: int32(1),
				MessageCount: int32(len(messages)),
				Results: []*mailboxv1.FetchMailboxInboxResult{{
					Mailbox:  publicMailbox(resultMailbox),
					Messages: messages,
				}},
			}, nil
		}
		fetchMailbox, err := s.store.PollMailboxForEmail(ctx, requestedEmail)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		resultMailbox := fetchMailbox
		if mailbox, err := s.store.FindMailbox(ctx, requestedEmail); err == nil {
			resultMailbox = mailbox
		}
		targets = append(targets, inboxTarget{fetchMailbox: fetchMailbox, resultMailbox: resultMailbox})
	} else {
		mailboxes, err := s.store.ListOAuthMailboxes(ctx, request.GetMaxMailboxes())
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		for _, mailbox := range mailboxes {
			targets = append(targets, inboxTarget{fetchMailbox: mailbox, resultMailbox: mailbox})
		}
	}

	resp := &mailboxv1.FetchMailboxInboxesResponse{
		MailboxCount: int32(len(targets)),
		Results:      []*mailboxv1.FetchMailboxInboxResult{},
	}
	for _, target := range targets {
		select {
		case <-ctx.Done():
			return nil, status.Error(codes.Canceled, "request cancelled")
		default:
		}

		result := &mailboxv1.FetchMailboxInboxResult{Mailbox: publicMailbox(target.resultMailbox)}
		messages, err := s.watcher.FetchMailboxInbox(ctx, target.fetchMailbox, request.GetLimitPerMailbox(), request.GetReceivedAfterUnix())
		if err != nil {
			result.ErrorMessage = err.Error()
			if cached, cacheErr := s.store.ListInboxMessagesSince(ctx, target.fetchMailbox.GetEmailAddress(), request.GetLimitPerMailbox(), request.GetReceivedAfterUnix()); cacheErr == nil {
				result.Messages = cached
				resp.MessageCount += int32(len(cached))
			}
			resp.FailedCount++
		} else {
			result.Messages = messages
			resp.FetchedCount++
			resp.MessageCount += int32(len(messages))
		}
		resp.Results = append(resp.Results, result)
	}
	return resp, nil
}

func (s *EmailService) ListInbox(ctx context.Context, request *mailboxv1.ListMailboxInboxRequest) (*mailboxv1.ListMailboxInboxResponse, error) {
	email := emailx.Normalize(request.GetEmailAddress())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email_address is required")
	}
	limit := request.GetLimit()
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	messages, err := s.store.ListInboxMessages(ctx, email, limit)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	resultMailbox := &pb.EmailMailbox{
		EmailAddress: email,
		ProviderKey:  s.providers.ProviderForInboxAddress(email, messages),
		Domain:       domainForEmail(email),
	}
	prepareMailboxProjection(resultMailbox)
	if mailbox, err := s.store.FindMailbox(ctx, email); err == nil {
		resultMailbox = mailbox
	}
	return &mailboxv1.ListMailboxInboxResponse{Result: &mailboxv1.FetchMailboxInboxResult{
		Mailbox:  publicMailbox(resultMailbox),
		Messages: messages,
	}}, nil
}

func (s *EmailService) WaitForEmail(ctx context.Context, request *mailboxv1.WaitForMailboxEmailRequest) (*mailboxv1.WaitForMailboxEmailResponse, error) {
	timeoutSeconds := request.GetTimeoutSeconds()
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	email := request.GetEmailAddress()
	issuedAfterUnix := request.GetIssuedAfterUnix()
	logInfo("waiting for email message email=%s timeout_seconds=%d issued_after_unix=%d", emailx.Redact(email), timeoutSeconds, issuedAfterUnix)
	if resp, ok, err := s.latestEmailResponse(ctx, request, issuedAfterUnix); err != nil {
		return nil, waitError(ctx, err)
	} else if ok {
		return resp, nil
	}
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	if !s.providers.IsStoredInboxOnlyAddress(email) {
		if s.work == nil {
			logWarning("mailbox email poll dispatcher is not configured email=%s", emailx.Redact(email))
		} else if err := s.work.PublishEmailPollRequested(ctx, &mailboxv1.MailboxEmailPollRequest{
			EmailAddress:    email,
			SubjectKeyword:  strings.TrimSpace(request.GetSubjectKeyword()),
			ParserProfile:   strings.TrimSpace(request.GetParserProfile()),
			SignalKind:      request.GetSignalKind(),
			IssuedAfterUnix: issuedAfterUnix,
			DeadlineUnix:    deadline.Unix(),
			Reason:          "wait_for_email",
		}); err != nil {
			return nil, waitError(ctx, err)
		}
	}
	return s.waitForPersistedEmail(ctx, request, timeoutSeconds, issuedAfterUnix)
}

func (s *EmailService) waitForPersistedEmail(ctx context.Context, request *mailboxv1.WaitForMailboxEmailRequest, timeoutSeconds int32, issuedAfterUnix int64) (*mailboxv1.WaitForMailboxEmailResponse, error) {
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	for time.Now().Before(deadline) {
		sleepFor := time.Duration(s.watcher.pollInterval) * time.Second
		if remaining := time.Until(deadline); remaining < sleepFor {
			sleepFor = remaining
		}
		if sleepFor > 0 {
			timer := time.NewTimer(sleepFor)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, status.Error(codes.Canceled, "request cancelled")
			case <-timer.C:
			}
		}
		if resp, ok, err := s.latestEmailResponse(ctx, request, issuedAfterUnix); err != nil {
			return nil, waitError(ctx, err)
		} else if ok {
			return resp, nil
		}
	}
	logInfo("webhook-backed email message not found email=%s timeout_seconds=%d issued_after_unix=%d", emailx.Redact(request.GetEmailAddress()), timeoutSeconds, issuedAfterUnix)
	return &mailboxv1.WaitForMailboxEmailResponse{Found: false}, nil
}

func (s *EmailService) latestEmailResponse(ctx context.Context, request *mailboxv1.WaitForMailboxEmailRequest, issuedAfterUnix int64) (*mailboxv1.WaitForMailboxEmailResponse, bool, error) {
	message, ok, err := s.store.LatestMessageWithSignal(ctx, request.GetEmailAddress(), request.GetSubjectKeyword(), issuedAfterUnix, request.GetParserProfile(), request.GetSignalKind())
	if err != nil || !ok {
		return nil, false, err
	}
	logInfo("served persisted email for %s provider=%s received_at_unix=%d", emailx.Redact(request.GetEmailAddress()), message.GetProviderKey(), message.GetReceivedAtUnix())
	return &mailboxv1.WaitForMailboxEmailResponse{Found: true, Message: message}, true, nil
}

func waitError(ctx context.Context, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return status.Error(codes.Canceled, "request cancelled")
	}
	return status.Error(codes.Internal, err.Error())
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not authorized") ||
		strings.Contains(msg, "no refresh token") ||
		strings.Contains(msg, "AUTH_FAILED")
}
