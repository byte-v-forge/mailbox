package app

import (
	"context"
	"time"

	"github.com/byte-v-forge/mailbox/internal/core"
)

const (
	defaultWaitTimeout  = 30 * time.Second
	defaultPollInterval = 2 * time.Second
)

type GetMessageOptions struct {
	IncludeHeaders     bool
	IncludeBody        bool
	IncludeAttachments bool
}

type MailboxService struct {
	accounts core.AccountStore
	messages core.MessageStore
	clock    core.Clock
}

func NewMailboxService(accounts core.AccountStore, messages core.MessageStore, clock core.Clock) *MailboxService {
	if clock == nil {
		clock = SystemClock{}
	}
	return &MailboxService{
		accounts: accounts,
		messages: messages,
		clock:    clock,
	}
}

func (s *MailboxService) GetEmailAccount(ctx context.Context, accountID string) (core.Account, error) {
	if accountID == "" {
		return core.Account{}, core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	return s.accounts.GetAccount(ctx, accountID)
}

func (s *MailboxService) ListMailFolders(ctx context.Context, accountID string) ([]core.Folder, error) {
	if accountID == "" {
		return nil, core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	return s.accounts.ListFolders(ctx, accountID)
}

func (s *MailboxService) SearchMessages(ctx context.Context, criteria core.SearchCriteria, pageSize int, pageToken string) (core.SearchResult, error) {
	if criteria.AccountID == "" {
		return core.SearchResult{}, core.NewError(core.CodeValidationFailed, "criteria.account_id is required", false)
	}
	if _, err := s.accounts.GetAccount(ctx, criteria.AccountID); err != nil {
		return core.SearchResult{}, err
	}
	return s.messages.SearchMessages(ctx, criteria, pageSize, pageToken)
}

func (s *MailboxService) GetMessage(ctx context.Context, accountID, emailID string, options GetMessageOptions) (core.Message, error) {
	if accountID == "" {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	if emailID == "" {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "email_id is required", false)
	}
	message, err := s.messages.GetMessage(ctx, accountID, emailID)
	if err != nil {
		return core.Message{}, err
	}
	return applyMessageOptions(message, options), nil
}

func (s *MailboxService) WaitForMessage(ctx context.Context, criteria core.SearchCriteria, timeout, pollInterval time.Duration) (core.Message, error) {
	if criteria.AccountID == "" {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "criteria.account_id is required", false)
	}
	if timeout < 0 {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "timeout cannot be negative", false)
	}
	if pollInterval < 0 {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "poll_interval cannot be negative", false)
	}
	if timeout == 0 {
		timeout = defaultWaitTimeout
	}
	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	deadline := s.clock.Now().Add(timeout)
	for {
		result, err := s.SearchMessages(ctx, criteria, 1, "")
		if err != nil {
			return core.Message{}, err
		}
		if len(result.Messages) > 0 {
			identity := result.Messages[0].Identity
			return s.GetMessage(ctx, identity.AccountID, identity.EmailID, GetMessageOptions{
				IncludeHeaders:     true,
				IncludeBody:        true,
				IncludeAttachments: true,
			})
		}
		if !s.clock.Now().Before(deadline) {
			return core.Message{}, core.NewError(core.CodeTimeout, "message wait timed out", true)
		}
		wait := pollInterval
		remaining := deadline.Sub(s.clock.Now())
		if remaining > 0 && remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return core.Message{}, core.NewError(core.CodeTimeout, ctx.Err().Error(), true)
		case <-timer.C:
		}
	}
}

func (s *MailboxService) AddMessageFlags(ctx context.Context, accountID, emailID string, flags []core.Flag) (core.MessageSummary, error) {
	flags = normalizeFlags(flags)
	if len(flags) == 0 {
		return core.MessageSummary{}, core.NewError(core.CodeValidationFailed, "flags are required", false)
	}
	message, err := s.messages.GetMessage(ctx, accountID, emailID)
	if err != nil {
		return core.MessageSummary{}, err
	}
	message.Flags = addFlags(message.Flags, flags)
	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		return core.MessageSummary{}, err
	}
	return message.Summary(), nil
}

func (s *MailboxService) RemoveMessageFlags(ctx context.Context, accountID, emailID string, flags []core.Flag) (core.MessageSummary, error) {
	flags = normalizeFlags(flags)
	if len(flags) == 0 {
		return core.MessageSummary{}, core.NewError(core.CodeValidationFailed, "flags are required", false)
	}
	message, err := s.messages.GetMessage(ctx, accountID, emailID)
	if err != nil {
		return core.MessageSummary{}, err
	}
	message.Flags = removeFlags(message.Flags, flags)
	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		return core.MessageSummary{}, err
	}
	return message.Summary(), nil
}

func (s *MailboxService) MoveMessage(ctx context.Context, accountID, emailID, targetFolderID string) (core.MessageSummary, error) {
	if targetFolderID == "" {
		return core.MessageSummary{}, core.NewError(core.CodeValidationFailed, "target_folder_id is required", false)
	}
	folders, err := s.accounts.ListFolders(ctx, accountID)
	if err != nil {
		return core.MessageSummary{}, err
	}
	if !folderExists(folders, targetFolderID) {
		return core.MessageSummary{}, core.NewError(core.CodeFolderNotFound, "target folder not found", false)
	}
	message, err := s.messages.GetMessage(ctx, accountID, emailID)
	if err != nil {
		return core.MessageSummary{}, err
	}
	message.Identity.FolderID = targetFolderID
	if err := s.messages.UpdateMessage(ctx, message); err != nil {
		return core.MessageSummary{}, err
	}
	return message.Summary(), nil
}

func (s *MailboxService) DeleteMessage(ctx context.Context, accountID, emailID string, permanent bool) error {
	return s.messages.DeleteMessage(ctx, accountID, emailID, permanent)
}

func applyMessageOptions(message core.Message, options GetMessageOptions) core.Message {
	if !options.IncludeHeaders {
		message.Headers = nil
	}
	if !options.IncludeBody {
		message.TextBody = ""
		message.HTMLBody = ""
	}
	if !options.IncludeAttachments {
		message.Attachments = nil
	}
	return message
}

func folderExists(folders []core.Folder, folderID string) bool {
	for _, folder := range folders {
		if folder.ID == folderID {
			return true
		}
	}
	return false
}

func addFlags(existing, incoming []core.Flag) []core.Flag {
	flags := normalizeFlags(existing)
	seen := make(map[core.Flag]struct{}, len(flags)+len(incoming))
	for _, flag := range flags {
		seen[flag] = struct{}{}
	}
	for _, flag := range incoming {
		if flag == "" {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		flags = append(flags, flag)
	}
	return flags
}

func removeFlags(existing, removing []core.Flag) []core.Flag {
	remove := make(map[core.Flag]struct{}, len(removing))
	for _, flag := range removing {
		remove[flag] = struct{}{}
	}
	out := make([]core.Flag, 0, len(existing))
	seen := make(map[core.Flag]struct{}, len(existing))
	for _, flag := range existing {
		if flag == "" {
			continue
		}
		if _, ok := remove[flag]; ok {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		out = append(out, flag)
	}
	return out
}

func normalizeFlags(flags []core.Flag) []core.Flag {
	out := make([]core.Flag, 0, len(flags))
	seen := make(map[core.Flag]struct{}, len(flags))
	for _, flag := range flags {
		if flag == "" {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		out = append(out, flag)
	}
	return out
}

func hasFlag(flags []core.Flag, target core.Flag) bool {
	for _, flag := range flags {
		if flag == target {
			return true
		}
	}
	return false
}
