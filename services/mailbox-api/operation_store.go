package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/dbclaim"
	"github.com/byte-v-forge/common-lib/emailx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

const (
	operationActionRunLeaseSeconds int32 = 2 * 60 * 60
)

var (
	errOperationAlreadyRunning = errors.New("mailbox operation is already running")
	errOperationInvalidAction  = errors.New("mailbox operation action mismatch")
)

type mailboxOperationRow struct {
	OperationID  string `gorm:"primaryKey;column:operation_id"`
	Action       string `gorm:"index"`
	Status       string `gorm:"index"`
	EmailAddress string `gorm:"index"`
	LastStep     string
	ErrorMessage string
	ImportOnly   bool
	OnlyMissing  bool
	Limit        int32
	ClaimOwner   string `gorm:"index"`
	ClaimUntil   int64  `gorm:"index"`
	AttemptCount int32
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

type operationRunStart struct {
	Operation    *mailboxv1.MailboxOperation
	EmailAddress string
	ImportOnly   bool
	OnlyMissing  bool
	Limit        int32
	Final        bool
}

func newOperationStore(dsn string) (*operationStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if !db.Migrator().HasTable((&mailboxOperationRow{}).TableName()) {
		return nil, errors.New("database schema is not migrated: missing table mailbox_operations")
	}
	return &operationStore{db: db}, nil
}

func (s *operationStore) create(ctx context.Context, operationID, action, emailAddress string) (*mailboxv1.MailboxOperation, error) {
	row := &mailboxOperationRow{
		OperationID:  strings.TrimSpace(operationID),
		Action:       strings.ToUpper(strings.TrimSpace(action)),
		Status:       operationStatusCreated,
		EmailAddress: emailx.Normalize(emailAddress),
		LastStep:     "created",
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(row), nil
}

func (s *operationStore) createRegistration(ctx context.Context, operationID string, importOnly bool) (*mailboxv1.MailboxOperation, error) {
	row := &mailboxOperationRow{
		OperationID: strings.TrimSpace(operationID),
		Action:      operationActionRegisterMailbox,
		Status:      operationStatusCreated,
		LastStep:    "queued",
		ImportOnly:  importOnly,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(row), nil
}

func (s *operationStore) createOAuth(ctx context.Context, operationID string, emailAddress string, onlyMissing bool, limit int32) (*mailboxv1.MailboxOperation, error) {
	row := &mailboxOperationRow{
		OperationID:  strings.TrimSpace(operationID),
		Action:       operationActionMailboxOAuth,
		Status:       operationStatusCreated,
		EmailAddress: emailx.Normalize(emailAddress),
		LastStep:     "queued",
		OnlyMissing:  onlyMissing,
		Limit:        normalizedLimit(limit),
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(row), nil
}

func (s *operationStore) startRegistrationWorkerRun(ctx context.Context, operationID string) (*operationRunStart, error) {
	return s.startWorkerRun(ctx, operationID, operationActionRegisterMailbox, "run_registration", "mailbox-registration-worker")
}

func (s *operationStore) startOAuthWorkerRun(ctx context.Context, operationID string) (*operationRunStart, error) {
	return s.startWorkerRun(ctx, operationID, operationActionMailboxOAuth, "run_oauth", "mailbox-oauth-worker")
}

func (s *operationStore) startWorkerRun(ctx context.Context, operationID string, action string, runStep string, workerID string) (*operationRunStart, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil, errors.New("operation_id is required")
	}
	now := time.Now().Unix()
	runLeaseUntil := dbclaim.Until(now, operationActionRunLeaseSeconds)
	action = strings.ToUpper(strings.TrimSpace(action))
	workerID = strings.TrimSpace(workerID)

	var row mailboxOperationRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(dbclaim.ForUpdate()).First(&row, "operation_id = ?", operationID).Error; err != nil {
			return err
		}
		if row.Action != action {
			return errOperationInvalidAction
		}
		switch row.Status {
		case operationStatusSucceeded, operationStatusFailed:
			return nil
		case operationStatusRunning:
			if row.LastStep == runStep && row.ClaimUntil > now {
				return errOperationAlreadyRunning
			}
		}
		if err := tx.Model(&mailboxOperationRow{}).
			Where("operation_id = ?", operationID).
			Updates(dbclaim.ClaimUpdates(operationStatusRunning, runStep, "", workerID, runLeaseUntil)).Error; err != nil {
			return err
		}
		return tx.First(&row, "operation_id = ?", operationID).Error
	})
	if err != nil {
		return nil, err
	}
	return &operationRunStart{
		Operation:    operationRowToProto(&row),
		EmailAddress: row.EmailAddress,
		ImportOnly:   row.ImportOnly,
		OnlyMissing:  row.OnlyMissing,
		Limit:        row.Limit,
		Final:        row.Status == operationStatusSucceeded || row.Status == operationStatusFailed,
	}, nil
}

func (s *operationStore) update(ctx context.Context, operationID string, update operationUpdate) (*mailboxv1.MailboxOperation, error) {
	updates := map[string]any{}
	if value := strings.ToUpper(strings.TrimSpace(update.Status)); value != "" {
		updates["status"] = value
		if value == operationStatusSucceeded || value == operationStatusFailed {
			for key, item := range dbclaim.ClearUpdates() {
				updates[key] = item
			}
		}
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

func (s *operationStore) get(ctx context.Context, operationID string) (*mailboxv1.MailboxOperation, error) {
	var row mailboxOperationRow
	if err := s.db.WithContext(ctx).First(&row, "operation_id = ?", strings.TrimSpace(operationID)).Error; err != nil {
		return nil, err
	}
	return operationRowToProto(&row), nil
}

func (s *operationStore) list(ctx context.Context, filter operationListFilter) ([]*mailboxv1.MailboxOperation, error) {
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
	if value := emailx.Normalize(filter.EmailAddress); value != "" {
		query = query.Where("email_address = ?", value)
	}

	var rows []mailboxOperationRow
	if err := query.Order("updated_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	operations := make([]*mailboxv1.MailboxOperation, 0, len(rows))
	for i := range rows {
		operations = append(operations, operationRowToProto(&rows[i]))
	}
	return operations, nil
}

func operationRowToProto(row *mailboxOperationRow) *mailboxv1.MailboxOperation {
	if row == nil {
		return nil
	}
	return &mailboxv1.MailboxOperation{
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
