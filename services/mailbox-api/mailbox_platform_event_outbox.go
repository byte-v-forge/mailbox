package main

import (
	"context"
	"fmt"
	"time"

	"github.com/byte-v-forge/common-lib/eventbus"
	"github.com/byte-v-forge/common-lib/eventoutbox"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/jackc/pgx/v5"
)

const mailboxPlatformEventOutboxTable = "mailbox_platform_event_outbox"

func (s *MailboxStore) enqueueInboxOutboxEvents(ctx context.Context, tx pgx.Tx, messages []*mailboxv1.EmailInboxMessage) error {
	if len(messages) == 0 {
		return nil
	}
	return enqueueMailboxOutboxEvents(ctx, tx, mailboxPlatformEventMessages(mailboxPlatformEventSource, messages))
}

func enqueueMailboxOutboxEvents(ctx context.Context, tx pgx.Tx, messages []eventbus.Message) error {
	for _, message := range messages {
		record, err := eventoutbox.NewRecord(message)
		if err != nil {
			return fmt.Errorf("prepare mailbox outbox event: %w", err)
		}
		if err := eventoutbox.InsertRecordPgx(ctx, tx, mailboxPlatformEventOutboxTable, record, time.Now().Unix()); err != nil {
			return err
		}
	}
	return nil
}

type mailboxPlatformEventOutboxWorker struct {
	store     *MailboxStore
	publisher *mailboxPlatformEvents
}

func runMailboxPlatformEventOutboxWorker(ctx context.Context, store *MailboxStore, publisher *mailboxPlatformEvents) error {
	if store == nil || publisher == nil {
		return nil
	}
	return eventoutbox.RunWorker(ctx, eventoutbox.WorkerConfig{
		Name:      "mailbox platform event outbox",
		Processor: &mailboxPlatformEventOutboxWorker{store: store, publisher: publisher},
		Logf:      logWarning,
	})
}

func (w *mailboxPlatformEventOutboxWorker) PublishPending(ctx context.Context, batch int) (int, error) {
	if w == nil || w.store == nil || w.publisher == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = eventoutbox.DefaultBatch
	}
	tx, err := w.store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	rows, err := eventoutbox.ClaimPendingPgx(ctx, tx, mailboxPlatformEventOutboxTable, batch, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	updates, err := eventoutbox.NewPgxUpdates(tx, mailboxPlatformEventOutboxTable)
	if err != nil {
		return 0, err
	}
	published, err := eventoutbox.PublishRows(ctx, w.publisher, rows, updates, eventoutbox.PublishOptions{})
	if err != nil {
		return published, err
	}
	if err := tx.Commit(ctx); err != nil {
		return published, err
	}
	committed = true
	return published, nil
}
