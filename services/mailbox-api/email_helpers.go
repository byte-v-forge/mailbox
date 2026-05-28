package main

import (
	"log"
	"regexp"
	"strings"
)

const (
	defaultListenAddr          = ":50051"
	defaultWebhookTokenHeader  = "X-Webhook-Token"
	defaultOAuthClientID       = "9e5f94bc-e8a4-4e73-b8be-63364c29d753"
	defaultOAuthScope          = "https://graph.microsoft.com/Mail.Read"
	defaultTokenURL            = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	defaultGraphMessagesURL    = "https://graph.microsoft.com/v1.0/me/messages"
	defaultPollIntervalSeconds = 5
	defaultMessageLimit        = 25
	defaultHTTPTimeoutSeconds  = 20
	defaultInboxOverlapSeconds = 120
	defaultWebhookMaxMailboxes = 100
	defaultWebhookTimeout      = 60
	defaultOutlookMaxMessages  = 100
	defaultCloudflareMaxDomain = 500
)

const (
	emailProviderOutlook    = "outlook"
	emailProviderCloudflare = "cloudflare"
)

const (
	authStatusAuthorized        = "AUTHORIZED"
	authStatusOAuthPending      = "OAUTH_PENDING"
	authStatusAuthFailed        = "AUTH_FAILED"
	authStatusNeedsManualVerify = "NEEDS_MANUAL_VERIFICATION"
)

var (
	emailPattern   = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
)

func logInfo(format string, args ...any) {
	log.Printf("[MAIL] "+format, args...)
}

func logWarning(format string, args ...any) {
	log.Printf("[MAIL] WARNING "+format, args...)
}

func normalizeScope(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(value, ",", " ")), " ")
}
