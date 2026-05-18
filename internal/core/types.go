package core

import (
	"fmt"
	"time"
)

type ErrorCode string

const (
	CodeValidationFailed     ErrorCode = "validation_failed"
	CodeAccountNotFound      ErrorCode = "account_not_found"
	CodeFolderNotFound       ErrorCode = "folder_not_found"
	CodeMessageNotFound      ErrorCode = "message_not_found"
	CodeAuthenticationFailed ErrorCode = "authentication_failed"
	CodePermissionDenied     ErrorCode = "permission_denied"
	CodeRateLimited          ErrorCode = "rate_limited"
	CodeProviderUnavailable  ErrorCode = "provider_unavailable"
	CodeTimeout              ErrorCode = "timeout"
	CodeUnsupportedOperation ErrorCode = "unsupported_operation"
	CodeMessageExpired       ErrorCode = "message_expired"
	CodeInternal             ErrorCode = "internal"
)

type Error struct {
	Code      ErrorCode
	Message   string
	Retryable bool
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code ErrorCode, message string, retryable bool) *Error {
	return &Error{Code: code, Message: message, Retryable: retryable}
}

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusDisabled  AccountStatus = "disabled"
	AccountStatusExpired   AccountStatus = "expired"
	AccountStatusFailed    AccountStatus = "failed"
)

type FolderRole string

const (
	FolderRoleInbox   FolderRole = "inbox"
	FolderRoleArchive FolderRole = "archive"
	FolderRoleSent    FolderRole = "sent"
	FolderRoleDrafts  FolderRole = "drafts"
	FolderRoleTrash   FolderRole = "trash"
	FolderRoleJunk    FolderRole = "junk"
	FolderRoleCustom  FolderRole = "custom"
)

type Flag string

const (
	FlagSeen      Flag = "seen"
	FlagAnswered  Flag = "answered"
	FlagFlagged   Flag = "flagged"
	FlagDeleted   Flag = "deleted"
	FlagDraft     Flag = "draft"
	FlagRecent    Flag = "recent"
	FlagImportant Flag = "important"
)

type Address struct {
	Email       string
	DisplayName string
}

type Account struct {
	ID             string
	PrimaryAddress Address
	Aliases        []Address
	Status         AccountStatus
	Labels         map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Folder struct {
	ID             string
	AccountID      string
	Name           string
	Role           FolderRole
	ParentFolderID string
	TotalMessages  int64
	UnreadMessages int64
	Labels         map[string]string
	UpdatedAt      time.Time
}

type MessageIdentity struct {
	EmailID         string
	AccountID       string
	FolderID        string
	ThreadID        string
	RFCMessageID    string
	SourceMessageID string
	UID             uint64
	UIDValidity     uint64
}

type Header struct {
	Name  string
	Value string
}

type Attachment struct {
	ID          string
	Filename    string
	ContentType string
	SizeBytes   int64
	ContentID   string
	Inline      bool
	Disposition string
}

type MessageSummary struct {
	Identity       MessageIdentity
	From           Address
	To             []Address
	Subject        string
	Snippet        string
	SentAt         time.Time
	ReceivedAt     time.Time
	Flags          []Flag
	HasAttachments bool
	SizeBytes      int64
	Labels         map[string]string
}

type Message struct {
	Identity    MessageIdentity
	From        Address
	To          []Address
	Cc          []Address
	Bcc         []Address
	ReplyTo     []Address
	Subject     string
	SentAt      time.Time
	ReceivedAt  time.Time
	Headers     []Header
	TextBody    string
	HTMLBody    string
	Snippet     string
	Flags       []Flag
	Attachments []Attachment
	SizeBytes   int64
	Labels      map[string]string
}

func (m Message) Summary() MessageSummary {
	return MessageSummary{
		Identity:       m.Identity,
		From:           m.From,
		To:             append([]Address(nil), m.To...),
		Subject:        m.Subject,
		Snippet:        m.Snippet,
		SentAt:         m.SentAt,
		ReceivedAt:     m.ReceivedAt,
		Flags:          append([]Flag(nil), m.Flags...),
		HasAttachments: len(m.Attachments) > 0,
		SizeBytes:      m.SizeBytes,
		Labels:         cloneMap(m.Labels),
	}
}

type SearchCriteria struct {
	AccountID           string
	FolderID            string
	FromEmail           string
	ToEmail             string
	SubjectContains     string
	BodyContains        string
	HeaderName          string
	HeaderValueContains string
	ReceivedAfter       time.Time
	ReceivedBefore      time.Time
	RequiredFlags       []Flag
	IncludeDeleted      bool
	Query               string
}

type SearchResult struct {
	Messages      []MessageSummary
	NextPageToken string
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
