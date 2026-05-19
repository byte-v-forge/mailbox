package main

import (
	"context"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"mailboxapi/pb"
)

const (
	operationActionRegisterMailbox = "REGISTER_MAILBOX"
	operationActionMailboxOAuth    = "MAILBOX_OAUTH"
	operationActionFetchInboxes    = "FETCH_INBOXES"

	operationStatusCreated   = "CREATED"
	operationStatusRunning   = "RUNNING"
	operationStatusSucceeded = "SUCCEEDED"
	operationStatusFailed    = "FAILED"
)

type mailboxOperationRow struct {
	OperationID  string `gorm:"primaryKey;column:operation_id"`
	Action       string `gorm:"index"`
	Status       string `gorm:"index"`
	EmailAddress string `gorm:"index"`
	LastStep     string
	ErrorMessage string
	ExitCode     int32
	MailboxCount int32
	FetchedCount int32
	FailedCount  int32
	MessageCount int32
	CreatedAt    int64 `gorm:"autoCreateTime"`
	UpdatedAt    int64 `gorm:"autoUpdateTime"`
}

func (mailboxOperationRow) TableName() string {
	return "mailbox_operations"
}

type operationStore struct {
	db *gorm.DB
}

type operationUpdate struct {
	Status       string
	LastStep     string
	ErrorMessage string
	ExitCode     int32
	MailboxCount int32
	FetchedCount int32
	FailedCount  int32
	MessageCount int32
}

type operationListFilter struct {
	Limit        int
	Status       string
	Action       string
	EmailAddress string
}

func newOperationStore(dsn string) (*operationStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&mailboxOperationRow{}); err != nil {
		return nil, err
	}
	return &operationStore{db: db}, nil
}

func (s *operationStore) create(ctx context.Context, operationID, action, emailAddress string) (*pb.MailboxOperation, error) {
	row := &mailboxOperationRow{
		OperationID:  strings.TrimSpace(operationID),
		Action:       strings.ToUpper(strings.TrimSpace(action)),
		Status:       operationStatusCreated,
		EmailAddress: normalizeEmail(emailAddress),
		LastStep:     "created",
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(row), nil
}

func (s *operationStore) update(ctx context.Context, operationID string, update operationUpdate) (*pb.MailboxOperation, error) {
	updates := map[string]any{}
	if value := strings.ToUpper(strings.TrimSpace(update.Status)); value != "" {
		updates["status"] = value
	}
	if value := strings.TrimSpace(update.LastStep); value != "" {
		updates["last_step"] = value
	}
	updates["error_message"] = strings.TrimSpace(update.ErrorMessage)
	updates["exit_code"] = update.ExitCode
	updates["mailbox_count"] = update.MailboxCount
	updates["fetched_count"] = update.FetchedCount
	updates["failed_count"] = update.FailedCount
	updates["message_count"] = update.MessageCount

	if err := s.db.WithContext(ctx).Model(&mailboxOperationRow{}).
		Where("operation_id = ?", strings.TrimSpace(operationID)).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.get(ctx, operationID)
}

func (s *operationStore) get(ctx context.Context, operationID string) (*pb.MailboxOperation, error) {
	var row mailboxOperationRow
	if err := s.db.WithContext(ctx).First(&row, "operation_id = ?", strings.TrimSpace(operationID)).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(&row), nil
}

func (s *operationStore) list(ctx context.Context, filter operationListFilter) ([]*pb.MailboxOperation, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := s.db.WithContext(ctx).Model(&mailboxOperationRow{})
	if value := strings.ToUpper(strings.TrimSpace(filter.Status)); value != "" {
		query = query.Where("status = ?", value)
	}
	if value := strings.ToUpper(strings.TrimSpace(filter.Action)); value != "" {
		query = query.Where("action = ?", value)
	}
	if value := normalizeEmail(filter.EmailAddress); value != "" {
		query = query.Where("email_address = ?", value)
	}

	var rows []mailboxOperationRow
	if err := query.Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	operations := make([]*pb.MailboxOperation, 0, len(rows))
	for i := range rows {
		operations = append(operations, operationRowToProto(&rows[i]))
	}
	return operations, nil
}

func operationRowToProto(row *mailboxOperationRow) *pb.MailboxOperation {
	if row == nil {
		return nil
	}
	return &pb.MailboxOperation{
		OperationId:  row.OperationID,
		Action:       row.Action,
		Status:       row.Status,
		EmailAddress: row.EmailAddress,
		LastStep:     row.LastStep,
		ErrorMessage: row.ErrorMessage,
		ExitCode:     row.ExitCode,
		MailboxCount: row.MailboxCount,
		FetchedCount: row.FetchedCount,
		FailedCount:  row.FailedCount,
		MessageCount: row.MessageCount,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
