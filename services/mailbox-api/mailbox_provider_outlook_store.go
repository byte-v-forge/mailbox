package main

import (
	"context"
	"errors"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	"github.com/byte-v-forge/common-lib/envx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/timex"

	"github.com/jackc/pgx/v5"
)

func (s *MailboxStore) RecordInboxMessages(ctx context.Context, email string, messages []graphMessage) ([]*mailboxv1.EmailInboxMessage, error) {
	email = emailx.Normalize(email)
	if email == "" {
		return nil, errors.New("email_address is required")
	}
	if len(messages) == 0 {
		return []*mailboxv1.EmailInboxMessage{}, nil
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now := time.Now().Unix()
	unseen := make([]*mailboxv1.EmailInboxMessage, 0, len(messages))
	var maxReceivedAtNs int64
	touchedMailboxes := map[string]struct{}{}
	for _, msg := range messages {
		receivedAtNs := timex.UnixNano(msg.ReceivedDateTime)
		if receivedAtNs > maxReceivedAtNs {
			maxReceivedAtNs = receivedAtNs
		}
		inboxMsg := inboxMessage(email, msg)
		key := stableMessageKey(emailProviderOutlook, email, messageKey(msg))
		persistedMessages := []*mailboxv1.EmailInboxMessage{}
		for _, mailboxEmail := range messageMailboxEmails(email, inboxMsg.GetRecipients()) {
			persisted := &mailboxv1.EmailInboxMessage{
				Id:                 inboxMsg.GetId(),
				MailboxEmail:       mailboxEmail,
				Subject:            inboxMsg.GetSubject(),
				FromAddress:        inboxMsg.GetFromAddress(),
				BodyPreview:        inboxMsg.GetBodyPreview(),
				ReceivedAtUnix:     inboxMsg.GetReceivedAtUnix(),
				Recipients:         inboxMsg.GetRecipients(),
				ProviderKey:        emailProviderOutlook,
				SourceMailboxEmail: email,
				BodyText:           inboxMsg.GetBodyText(),
				HtmlBody:           inboxMsg.GetHtmlBody(),
				RawSize:            inboxMsg.GetRawSize(),
			}
			persistedMessages = append(persistedMessages, emailMessageWithSignals(persisted, ""))
			touchedMailboxes[mailboxEmail] = struct{}{}
			if err := insertInboxMessage(ctx, tx, inboxPersistMessage{
				key:            stableMessageKey(emailProviderOutlook, mailboxEmail, messageKey(msg)),
				id:             inboxMsg.GetId(),
				mailboxEmail:   mailboxEmail,
				subject:        inboxMsg.GetSubject(),
				fromAddress:    inboxMsg.GetFromAddress(),
				bodyPreview:    inboxMsg.GetBodyPreview(),
				receivedAtUnix: inboxMsg.GetReceivedAtUnix(),
				recipients:     inboxMsg.GetRecipients(),
				provider:       emailProviderOutlook,
				sourceEmail:    email,
				bodyText:       inboxMsg.GetBodyText(),
				htmlBody:       inboxMsg.GetHtmlBody(),
				rawSize:        inboxMsg.GetRawSize(),
			}, now); err != nil {
				return nil, err
			}
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO mailbox_inbox_seen (provider, mailbox_email, message_key, seen_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (provider, mailbox_email, message_key) DO NOTHING
		`, emailProviderOutlook, email, key, now)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() > 0 {
			unseen = append(unseen, persistedMessages...)
		}
	}
	if maxReceivedAtNs > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE mailboxes
			SET last_inbox_received_at_ns = GREATEST(last_inbox_received_at_ns, $1),
				updated_at = $2
			WHERE email = $3
		`, maxReceivedAtNs, now, email); err != nil {
			return nil, err
		}
	}
	for mailboxEmail := range touchedMailboxes {
		if err := pruneMailboxMessages(ctx, tx, emailProviderOutlook, mailboxEmail, envx.Int("MAILBOX_OUTLOOK_MAX_MESSAGES_PER_MAILBOX", defaultOutlookMaxMessages)); err != nil {
			return nil, err
		}
	}
	if err := s.enqueueInboxOutboxEvents(ctx, tx, unseen); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	s.recordRecentInboxMessages(ctx, unseen)
	return unseen, nil
}
