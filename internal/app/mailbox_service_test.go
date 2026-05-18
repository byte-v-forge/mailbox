package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/byte-v-forge/mailbox/internal/core"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func TestSearchMessagesFiltersByCoreCriteria(t *testing.T) {
	ctx := context.Background()
	service := newTestService()

	result, err := service.SearchMessages(ctx, core.SearchCriteria{
		AccountID:       "acct-1",
		FolderID:        "inbox",
		FromEmail:       "sender@example.com",
		SubjectContains: "code",
		RequiredFlags:   []core.Flag{core.FlagRecent},
		Query:           "482910",
	}, 10, "")
	if err != nil {
		t.Fatalf("SearchMessages() error = %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(result.Messages))
	}
	if result.Messages[0].Identity.EmailID != "msg-1" {
		t.Fatalf("email_id = %q, want msg-1", result.Messages[0].Identity.EmailID)
	}
}

func TestGetMessageHonorsIncludeOptions(t *testing.T) {
	ctx := context.Background()
	service := newTestService()

	message, err := service.GetMessage(ctx, "acct-1", "msg-1", GetMessageOptions{})
	if err != nil {
		t.Fatalf("GetMessage() error = %v", err)
	}
	if message.TextBody != "" || message.HTMLBody != "" {
		t.Fatal("GetMessage() returned body when IncludeBody is false")
	}
	if len(message.Headers) != 0 {
		t.Fatal("GetMessage() returned headers when IncludeHeaders is false")
	}
	if len(message.Attachments) != 0 {
		t.Fatal("GetMessage() returned attachments when IncludeAttachments is false")
	}
}

func TestWaitForMessageReturnsFirstMatch(t *testing.T) {
	ctx := context.Background()
	service := newTestService()

	message, err := service.WaitForMessage(ctx, core.SearchCriteria{
		AccountID: "acct-1",
		Query:     "482910",
	}, time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForMessage() error = %v", err)
	}
	if message.Identity.EmailID != "msg-1" {
		t.Fatalf("email_id = %q, want msg-1", message.Identity.EmailID)
	}
	if message.TextBody == "" {
		t.Fatal("WaitForMessage() should return full message body")
	}
}

func TestFlagMoveAndDeleteMessage(t *testing.T) {
	ctx := context.Background()
	store := newTestStore()
	service := NewMailboxService(store, store, &fakeClock{now: baseTime()})

	summary, err := service.AddMessageFlags(ctx, "acct-1", "msg-1", []core.Flag{core.FlagSeen, core.FlagSeen})
	if err != nil {
		t.Fatalf("AddMessageFlags() error = %v", err)
	}
	if !hasFlag(summary.Flags, core.FlagSeen) {
		t.Fatal("AddMessageFlags() did not add seen flag")
	}

	summary, err = service.RemoveMessageFlags(ctx, "acct-1", "msg-1", []core.Flag{core.FlagRecent})
	if err != nil {
		t.Fatalf("RemoveMessageFlags() error = %v", err)
	}
	if hasFlag(summary.Flags, core.FlagRecent) {
		t.Fatal("RemoveMessageFlags() did not remove recent flag")
	}

	summary, err = service.MoveMessage(ctx, "acct-1", "msg-1", "archive")
	if err != nil {
		t.Fatalf("MoveMessage() error = %v", err)
	}
	if summary.Identity.FolderID != "archive" {
		t.Fatalf("folder_id = %q, want archive", summary.Identity.FolderID)
	}

	if err := service.DeleteMessage(ctx, "acct-1", "msg-1", false); err != nil {
		t.Fatalf("DeleteMessage() error = %v", err)
	}
	result, err := service.SearchMessages(ctx, core.SearchCriteria{AccountID: "acct-1", IncludeDeleted: false}, 10, "")
	if err != nil {
		t.Fatalf("SearchMessages() after delete error = %v", err)
	}
	for _, message := range result.Messages {
		if message.Identity.EmailID == "msg-1" {
			t.Fatal("soft-deleted message appeared without IncludeDeleted")
		}
	}
}

func TestMoveMessageRejectsUnknownFolder(t *testing.T) {
	ctx := context.Background()
	service := newTestService()

	_, err := service.MoveMessage(ctx, "acct-1", "msg-1", "missing")
	if err == nil {
		t.Fatal("MoveMessage() expected error")
	}
	var coreErr *core.Error
	if !errors.As(err, &coreErr) || coreErr.Code != core.CodeFolderNotFound {
		t.Fatalf("MoveMessage() error = %#v, want folder_not_found", err)
	}
}

func newTestService() *MailboxService {
	store := newTestStore()
	return NewMailboxService(store, store, &fakeClock{now: baseTime()})
}

func newTestStore() *MemoryStore {
	now := baseTime()
	return NewMemoryStoreWithData(
		[]core.Account{{
			ID:             "acct-1",
			PrimaryAddress: core.Address{Email: "inbox@example.com", DisplayName: "Inbox"},
			Status:         core.AccountStatusActive,
			CreatedAt:      now.Add(-24 * time.Hour),
			UpdatedAt:      now,
		}},
		[]core.Folder{
			{ID: "inbox", AccountID: "acct-1", Name: "Inbox", Role: core.FolderRoleInbox, TotalMessages: 2, UnreadMessages: 1, UpdatedAt: now},
			{ID: "archive", AccountID: "acct-1", Name: "Archive", Role: core.FolderRoleArchive, UpdatedAt: now},
		},
		[]core.Message{
			{
				Identity: core.MessageIdentity{
					EmailID:      "msg-1",
					AccountID:    "acct-1",
					FolderID:     "inbox",
					ThreadID:     "thread-1",
					RFCMessageID: "<msg-1@example.com>",
				},
				From:       core.Address{Email: "sender@example.com", DisplayName: "Sender"},
				To:         []core.Address{{Email: "inbox@example.com"}},
				Subject:    "Your login code",
				SentAt:     now.Add(-time.Minute),
				ReceivedAt: now,
				Headers:    []core.Header{{Name: "X-Provider", Value: "imap"}},
				TextBody:   "Code: 482910",
				HTMLBody:   "<p>Code: 482910</p>",
				Snippet:    "Code: 482910",
				Flags:      []core.Flag{core.FlagRecent},
				Attachments: []core.Attachment{{
					ID:          "att-1",
					Filename:    "notice.txt",
					ContentType: "text/plain",
					SizeBytes:   12,
				}},
				SizeBytes: 512,
			},
			{
				Identity: core.MessageIdentity{
					EmailID:   "msg-2",
					AccountID: "acct-1",
					FolderID:  "inbox",
					ThreadID:  "thread-2",
				},
				From:       core.Address{Email: "newsletter@example.com"},
				To:         []core.Address{{Email: "inbox@example.com"}},
				Subject:    "Weekly update",
				ReceivedAt: now.Add(-time.Hour),
				TextBody:   "Nothing to verify",
				Snippet:    "Nothing to verify",
				Flags:      []core.Flag{core.FlagSeen},
			},
		},
	)
}

func baseTime() time.Time {
	return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
}
