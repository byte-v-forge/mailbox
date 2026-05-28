package main

import (
	"regexp"
	"strings"

	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"
)

var (
	emailOTPContextPattern    = regexp.MustCompile(`(?i)(?:verification|security|login|one[- ]?time|otp|code|验证码|安全代码)[^0-9]{0,80}([0-9]{4,8})`)
	emailOTPStandalonePattern = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
)

func emailMessageWithSignals(message *mailboxv1.EmailInboxMessage, _ string) *mailboxv1.EmailInboxMessage {
	if message == nil {
		return nil
	}
	code, evidence := extractEmailOTP(message)
	if code == "" {
		message.Signals = nil
		message.PrimarySignal = nil
		return message
	}
	signal := &mailboxv1.EmailSignal{
		Kind:       mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_OTP,
		Code:       normalizeEmailOTP(code),
		Label:      "verification_code",
		Profile:    "generic",
		Parser:     "mailbox-email-otp",
		Confidence: 70,
		Evidence:   evidence,
	}
	message.Signals = []*mailboxv1.EmailSignal{signal}
	message.PrimarySignal = signal
	return message
}

func messageHasSignal(message *mailboxv1.EmailInboxMessage, kind mailboxv1.EmailSignalKind) bool {
	if message == nil {
		return false
	}
	if kind == mailboxv1.EmailSignalKind_EMAIL_SIGNAL_KIND_UNSPECIFIED {
		return true
	}
	if signal := message.GetPrimarySignal(); signal.GetKind() == kind && signal.GetCode() != "" {
		return true
	}
	for _, signal := range message.GetSignals() {
		if signal.GetKind() == kind && signal.GetCode() != "" {
			return true
		}
	}
	return false
}

func extractEmailOTP(message *mailboxv1.EmailInboxMessage) (string, string) {
	if message == nil {
		return "", ""
	}
	text := strings.Join([]string{
		message.GetSubject(),
		message.GetFromAddress(),
		message.GetBodyPreview(),
		message.GetBodyText(),
		message.GetHtmlBody(),
	}, "\n")
	if match := emailOTPContextPattern.FindStringSubmatch(text); len(match) >= 2 {
		return normalizeEmailOTP(match[1]), strings.TrimSpace(match[0])
	}
	if match := emailOTPStandalonePattern.FindStringSubmatch(text); len(match) >= 3 {
		return normalizeEmailOTP(match[2]), strings.TrimSpace(match[0])
	}
	return "", ""
}

func normalizeEmailOTP(value string) string {
	return strings.TrimSpace(strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "").Replace(value))
}
