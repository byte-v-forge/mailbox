package app

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/byte-v-forge/mailbox/internal/core"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type MemoryStore struct {
	mu       sync.RWMutex
	accounts map[string]core.Account
	folders  map[string]map[string]core.Folder
	messages map[string]map[string]core.Message
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		accounts: make(map[string]core.Account),
		folders:  make(map[string]map[string]core.Folder),
		messages: make(map[string]map[string]core.Message),
	}
}

func NewMemoryStoreWithData(accounts []core.Account, folders []core.Folder, messages []core.Message) *MemoryStore {
	store := NewMemoryStore()
	for _, account := range accounts {
		store.accounts[account.ID] = cloneAccount(account)
	}
	for _, folder := range folders {
		store.putFolder(folder)
	}
	for _, message := range messages {
		store.putMessage(message)
	}
	return store
}

func (s *MemoryStore) SeedAccount(account core.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[account.ID] = cloneAccount(account)
}

func (s *MemoryStore) SeedFolder(folder core.Folder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putFolder(folder)
}

func (s *MemoryStore) SeedMessage(message core.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putMessage(message)
}

func (s *MemoryStore) GetAccount(_ context.Context, accountID string) (core.Account, error) {
	if accountID == "" {
		return core.Account{}, core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[accountID]
	if !ok {
		return core.Account{}, core.NewError(core.CodeAccountNotFound, "email account not found", false)
	}
	return cloneAccount(account), nil
}

func (s *MemoryStore) ListFolders(ctx context.Context, accountID string) ([]core.Folder, error) {
	if _, err := s.GetAccount(ctx, accountID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	byAccount := s.folders[accountID]
	folders := make([]core.Folder, 0, len(byAccount))
	for _, folder := range byAccount {
		folders = append(folders, cloneFolder(folder))
	}
	sort.Slice(folders, func(i, j int) bool {
		if folders[i].Role != folders[j].Role {
			return folders[i].Role < folders[j].Role
		}
		if folders[i].Name != folders[j].Name {
			return folders[i].Name < folders[j].Name
		}
		return folders[i].ID < folders[j].ID
	})
	return folders, nil
}

func (s *MemoryStore) SearchMessages(ctx context.Context, criteria core.SearchCriteria, pageSize int, pageToken string) (core.SearchResult, error) {
	if criteria.AccountID != "" {
		if _, err := s.GetAccount(ctx, criteria.AccountID); err != nil {
			return core.SearchResult{}, err
		}
	}
	offset, err := parsePageToken(pageToken)
	if err != nil {
		return core.SearchResult{}, err
	}
	pageSize = normalizePageSize(pageSize)

	s.mu.RLock()
	defer s.mu.RUnlock()
	var matched []core.MessageSummary
	for accountID, byEmailID := range s.messages {
		if criteria.AccountID != "" && accountID != criteria.AccountID {
			continue
		}
		for _, message := range byEmailID {
			if messageMatches(criteria, message) {
				matched = append(matched, cloneSummary(message.Summary()))
			}
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if !matched[i].ReceivedAt.Equal(matched[j].ReceivedAt) {
			return matched[i].ReceivedAt.After(matched[j].ReceivedAt)
		}
		return matched[i].Identity.EmailID < matched[j].Identity.EmailID
	})
	if offset >= len(matched) {
		return core.SearchResult{}, nil
	}
	end := offset + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	nextPageToken := ""
	if end < len(matched) {
		nextPageToken = strconv.Itoa(end)
	}
	return core.SearchResult{
		Messages:      matched[offset:end],
		NextPageToken: nextPageToken,
	}, nil
}

func (s *MemoryStore) GetMessage(ctx context.Context, accountID, emailID string) (core.Message, error) {
	if accountID == "" {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	if emailID == "" {
		return core.Message{}, core.NewError(core.CodeValidationFailed, "email_id is required", false)
	}
	if _, err := s.GetAccount(ctx, accountID); err != nil {
		return core.Message{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	message, ok := s.messages[accountID][emailID]
	if !ok {
		return core.Message{}, core.NewError(core.CodeMessageNotFound, "email message not found", false)
	}
	return cloneMessage(message), nil
}

func (s *MemoryStore) SaveMessage(_ context.Context, message core.Message) error {
	if message.Identity.AccountID == "" {
		return core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	if message.Identity.EmailID == "" {
		return core.NewError(core.CodeValidationFailed, "email_id is required", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[message.Identity.AccountID]; !ok {
		return core.NewError(core.CodeAccountNotFound, "email account not found", false)
	}
	s.putMessage(message)
	return nil
}

func (s *MemoryStore) UpdateMessage(_ context.Context, message core.Message) error {
	if message.Identity.AccountID == "" {
		return core.NewError(core.CodeValidationFailed, "account_id is required", false)
	}
	if message.Identity.EmailID == "" {
		return core.NewError(core.CodeValidationFailed, "email_id is required", false)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byEmailID := s.messages[message.Identity.AccountID]
	if _, ok := byEmailID[message.Identity.EmailID]; !ok {
		return core.NewError(core.CodeMessageNotFound, "email message not found", false)
	}
	s.putMessage(message)
	return nil
}

func (s *MemoryStore) DeleteMessage(ctx context.Context, accountID, emailID string, permanent bool) error {
	message, err := s.GetMessage(ctx, accountID, emailID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if permanent {
		delete(s.messages[accountID], emailID)
		return nil
	}
	message.Flags = addFlags(message.Flags, []core.Flag{core.FlagDeleted})
	s.putMessage(message)
	return nil
}

func (s *MemoryStore) putFolder(folder core.Folder) {
	if s.folders[folder.AccountID] == nil {
		s.folders[folder.AccountID] = make(map[string]core.Folder)
	}
	s.folders[folder.AccountID][folder.ID] = cloneFolder(folder)
}

func (s *MemoryStore) putMessage(message core.Message) {
	accountID := message.Identity.AccountID
	if s.messages[accountID] == nil {
		s.messages[accountID] = make(map[string]core.Message)
	}
	s.messages[accountID][message.Identity.EmailID] = cloneMessage(message)
}

func parsePageToken(pageToken string) (int, error) {
	if pageToken == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(pageToken)
	if err != nil || offset < 0 {
		return 0, core.NewError(core.CodeValidationFailed, "page_token must be a non-negative offset", false)
	}
	return offset, nil
}

func normalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultPageSize
	}
	if pageSize > maxPageSize {
		return maxPageSize
	}
	return pageSize
}

func messageMatches(criteria core.SearchCriteria, message core.Message) bool {
	if criteria.FolderID != "" && message.Identity.FolderID != criteria.FolderID {
		return false
	}
	if !criteria.IncludeDeleted && hasFlag(message.Flags, core.FlagDeleted) {
		return false
	}
	if criteria.FromEmail != "" && !strings.EqualFold(message.From.Email, criteria.FromEmail) {
		return false
	}
	if criteria.ToEmail != "" && !addressesContain(message.To, criteria.ToEmail) {
		return false
	}
	if criteria.SubjectContains != "" && !containsFold(message.Subject, criteria.SubjectContains) {
		return false
	}
	if criteria.BodyContains != "" && !containsFold(message.TextBody+"\n"+message.HTMLBody, criteria.BodyContains) {
		return false
	}
	if criteria.HeaderName != "" && !headersMatch(message.Headers, criteria.HeaderName, criteria.HeaderValueContains) {
		return false
	}
	if !criteria.ReceivedAfter.IsZero() && message.ReceivedAt.Before(criteria.ReceivedAfter) {
		return false
	}
	if !criteria.ReceivedBefore.IsZero() && message.ReceivedAt.After(criteria.ReceivedBefore) {
		return false
	}
	for _, flag := range criteria.RequiredFlags {
		if !hasFlag(message.Flags, flag) {
			return false
		}
	}
	if criteria.Query != "" && !queryMatches(criteria.Query, message) {
		return false
	}
	return true
}

func addressesContain(addresses []core.Address, email string) bool {
	for _, address := range addresses {
		if strings.EqualFold(address.Email, email) {
			return true
		}
	}
	return false
}

func headersMatch(headers []core.Header, name, valueContains string) bool {
	for _, header := range headers {
		if !strings.EqualFold(header.Name, name) {
			continue
		}
		return valueContains == "" || containsFold(header.Value, valueContains)
	}
	return false
}

func queryMatches(query string, message core.Message) bool {
	values := []string{
		message.From.Email,
		message.From.DisplayName,
		message.Subject,
		message.Snippet,
		message.TextBody,
		message.HTMLBody,
	}
	for _, address := range message.To {
		values = append(values, address.Email, address.DisplayName)
	}
	for _, value := range values {
		if containsFold(value, query) {
			return true
		}
	}
	return false
}

func containsFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func cloneAccount(account core.Account) core.Account {
	account.Aliases = append([]core.Address(nil), account.Aliases...)
	account.Labels = cloneLabels(account.Labels)
	return account
}

func cloneFolder(folder core.Folder) core.Folder {
	folder.Labels = cloneLabels(folder.Labels)
	return folder
}

func cloneSummary(summary core.MessageSummary) core.MessageSummary {
	summary.To = append([]core.Address(nil), summary.To...)
	summary.Flags = append([]core.Flag(nil), summary.Flags...)
	summary.Labels = cloneLabels(summary.Labels)
	return summary
}

func cloneMessage(message core.Message) core.Message {
	message.To = append([]core.Address(nil), message.To...)
	message.Cc = append([]core.Address(nil), message.Cc...)
	message.Bcc = append([]core.Address(nil), message.Bcc...)
	message.ReplyTo = append([]core.Address(nil), message.ReplyTo...)
	message.Headers = append([]core.Header(nil), message.Headers...)
	message.Flags = append([]core.Flag(nil), message.Flags...)
	message.Attachments = append([]core.Attachment(nil), message.Attachments...)
	message.Labels = cloneLabels(message.Labels)
	return message
}

func cloneLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
