package main

import (
	"context"
	"strings"

	"github.com/byte-v-forge/common-lib/envx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mailboxapi/pb"
)

func cloudflareMailboxProvider() *mailboxProviderPlugin {
	return &mailboxProviderPlugin{
		key:             emailProviderCloudflare,
		aliases:         []string{"cf", "cloudflare-email-relay"},
		displayName:     "Cloudflare",
		storedInboxOnly: true,
		capabilities: func() *mailboxv1.MailboxProviderCapabilities {
			return &mailboxv1.MailboxProviderCapabilities{
				Key:         emailProviderCloudflare,
				DisplayName: "Cloudflare",
				Actions: []*mailboxv1.MailboxProviderActionCapability{
					{Action: mailboxv1.MailboxProviderAction_MAILBOX_PROVIDER_ACTION_RECEIVE_WEBHOOK},
					{Action: mailboxv1.MailboxProviderAction_MAILBOX_PROVIDER_ACTION_AUTO_CREATE_MAILBOX},
					{Action: mailboxv1.MailboxProviderAction_MAILBOX_PROVIDER_ACTION_SYNC_DOMAINS},
				},
				RetentionPolicy: &mailboxv1.MailboxMessageRetentionPolicy{
					Scope:       mailboxv1.MailboxMessageRetentionScope_MAILBOX_MESSAGE_RETENTION_SCOPE_DOMAIN,
					MaxMessages: int32(envx.Int("MAILBOX_CLOUDFLARE_MAX_MESSAGES_PER_DOMAIN", defaultCloudflareMaxDomain)),
				},
			}
		},
		loadDomains: loadCloudflareEmailDomains,
		domains: func(configured []string) []*mailboxv1.MailboxDomain {
			domains := make([]*mailboxv1.MailboxDomain, 0, len(configured))
			for _, domain := range configured {
				domains = append(domains, &mailboxv1.MailboxDomain{
					ProviderKey: emailProviderCloudflare,
					Domain:      domain,
					Enabled:     true,
				})
			}
			return domains
		},
		matchesAddress: func(email string, cfg mailboxProviderRuntimeConfig) bool {
			domain := domainForEmail(email)
			if domain == "" {
				return false
			}
			for _, candidate := range cfg.domainsForProvider(emailProviderCloudflare) {
				if domain == strings.Trim(strings.ToLower(strings.TrimSpace(candidate)), ".") {
					return true
				}
			}
			return false
		},
		pruneInbound: func(ctx context.Context, tx pgx.Tx, retention mailboxInboxRetention) error {
			for domain := range retention.touchedDomains {
				if err := pruneDomainMessages(ctx, tx, emailProviderCloudflare, domain, envx.Int("MAILBOX_CLOUDFLARE_MAX_MESSAGES_PER_DOMAIN", defaultCloudflareMaxDomain)); err != nil {
					return err
				}
			}
			return nil
		},
		includeVirtual: func(authStatus string) bool {
			return authStatus == ""
		},
		virtualMailboxes: listCloudflareVirtualMailboxes,
		prepareProjection: func(mailbox *pb.EmailMailbox) {
			mailbox.AuthStatus = ""
			mailbox.Password = ""
			mailbox.RefreshToken = ""
			mailbox.AccessToken = ""
			mailbox.LastError = ""
		},
	}
}

func listCloudflareVirtualMailboxes(ctx context.Context, pool *pgxpool.Pool, limit int) ([]*pb.EmailMailbox, error) {
	rows, err := pool.Query(ctx, `
		SELECT 'cloudflare:' || msg.mailbox_email, msg.mailbox_email,
			$1, '', '', '', '', '', MIN(msg.created_at), MAX(msg.updated_at)
		FROM mailbox_inbox_messages msg
		WHERE msg.provider = $1
		  AND NOT EXISTS (SELECT 1 FROM mailboxes m WHERE m.email = msg.mailbox_email)
		GROUP BY msg.mailbox_email
		ORDER BY MAX(msg.updated_at) DESC
		LIMIT $2
	`, emailProviderCloudflare, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*pb.EmailMailbox{}
	for rows.Next() {
		row, err := scanMailbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row.toProto())
	}
	return out, rows.Err()
}
