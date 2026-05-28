package main

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/byte-v-forge/common-lib/emailx"
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/common-lib/redisx"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type recentEmailCache struct {
	client      redis.Cmdable
	keyspace    redisx.Keyspace
	ttl         time.Duration
	maxMessages int64
}

func newRecentEmailCache(client redis.Cmdable, prefix string, ttl time.Duration, maxMessages int) *recentEmailCache {
	if maxMessages <= 0 {
		maxMessages = 20
	}
	return &recentEmailCache{
		client:      client,
		keyspace:    redisx.NewKeyspace(prefix),
		ttl:         ttl,
		maxMessages: int64(maxMessages),
	}
}

func (c *recentEmailCache) Record(ctx context.Context, messages []*mailboxv1.EmailInboxMessage) error {
	if c == nil || c.client == nil || len(messages) == 0 {
		return nil
	}
	byMailbox := map[string][]*mailboxv1.EmailInboxMessage{}
	for _, message := range messages {
		key, ok := c.key(message.GetMailboxEmail())
		if !ok {
			continue
		}
		byMailbox[key] = append(byMailbox[key], message)
	}
	if len(byMailbox) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	for key, messages := range byMailbox {
		sort.SliceStable(messages, func(i, j int) bool {
			return messages[i].GetReceivedAtUnix() < messages[j].GetReceivedAtUnix()
		})
		encoded := make([]any, 0, len(messages))
		for _, message := range messages {
			payload, ok := encodeRecentEmailMessage(message)
			if ok {
				encoded = append(encoded, payload)
			}
		}
		if len(encoded) == 0 {
			continue
		}
		pipe.LPush(ctx, key, encoded...)
		pipe.LTrim(ctx, key, 0, c.maxMessages-1)
		if c.ttl > 0 {
			pipe.Expire(ctx, key, c.ttl)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (c *recentEmailCache) Latest(ctx context.Context, email string, subjectKeyword string, issuedAfterUnix int64, parserProfile string, signalKind mailboxv1.EmailSignalKind) (*mailboxv1.EmailInboxMessage, bool, error) {
	key, ok := c.key(email)
	if !ok {
		return nil, false, nil
	}
	items, err := c.client.LRange(ctx, key, 0, c.maxMessages-1).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var best *mailboxv1.EmailInboxMessage
	for _, item := range items {
		message, ok := decodeRecentEmailMessage(item, parserProfile)
		if !ok || !recentEmailMatches(message, subjectKeyword, issuedAfterUnix, signalKind) {
			continue
		}
		if best == nil || message.GetReceivedAtUnix() > best.GetReceivedAtUnix() {
			best = message
		}
	}
	return best, best != nil, nil
}

func (c *recentEmailCache) key(email string) (string, bool) {
	email = emailx.Normalize(email)
	if email == "" || c == nil || c.client == nil {
		return "", false
	}
	return c.keyspace.Key(email)
}

func encodeRecentEmailMessage(message *mailboxv1.EmailInboxMessage) (string, bool) {
	if message == nil || emailx.Normalize(message.GetMailboxEmail()) == "" {
		return "", false
	}
	cloned, ok := proto.Clone(message).(*mailboxv1.EmailInboxMessage)
	if !ok {
		return "", false
	}
	cloned.MailboxEmail = emailx.Normalize(cloned.GetMailboxEmail())
	payload, err := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(cloned)
	if err != nil {
		return "", false
	}
	return string(payload), true
}

func decodeRecentEmailMessage(payload string, parserProfile string) (*mailboxv1.EmailInboxMessage, bool) {
	message := &mailboxv1.EmailInboxMessage{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(payload), message); err != nil {
		return nil, false
	}
	return emailMessageWithSignals(message, parserProfile), true
}

func recentEmailMatches(message *mailboxv1.EmailInboxMessage, subjectKeyword string, issuedAfterUnix int64, signalKind mailboxv1.EmailSignalKind) bool {
	if message == nil {
		return false
	}
	if issuedAfterUnix > 0 && message.GetReceivedAtUnix() < issuedAfterUnix {
		return false
	}
	if keyword := strings.ToLower(strings.TrimSpace(subjectKeyword)); keyword != "" && !recentEmailContainsKeyword(message, keyword) {
		return false
	}
	return messageHasSignal(message, signalKind)
}

func recentEmailContainsKeyword(message *mailboxv1.EmailInboxMessage, keyword string) bool {
	return strings.Contains(strings.ToLower(message.GetSubject()), keyword) ||
		strings.Contains(strings.ToLower(message.GetBodyPreview()), keyword) ||
		strings.Contains(strings.ToLower(message.GetBodyText()), keyword)
}
