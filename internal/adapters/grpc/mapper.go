package grpcadapter

import (
	"errors"
	"time"

	mailboxv1 "github.com/byte-v-forge/contracts-go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/mailbox/internal/core"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoAccount(account core.Account) *mailboxv1.EmailAccount {
	return &mailboxv1.EmailAccount{
		AccountId:      account.ID,
		PrimaryAddress: toProtoAddress(account.PrimaryAddress),
		Aliases:        toProtoAddresses(account.Aliases),
		Status:         toProtoAccountStatus(account.Status),
		Labels:         cloneMap(account.Labels),
		CreatedAt:      toProtoTime(account.CreatedAt),
		UpdatedAt:      toProtoTime(account.UpdatedAt),
	}
}

func toProtoFolder(folder core.Folder) *mailboxv1.MailFolder {
	return &mailboxv1.MailFolder{
		FolderId:       folder.ID,
		AccountId:      folder.AccountID,
		Name:           folder.Name,
		Role:           toProtoFolderRole(folder.Role),
		ParentFolderId: folder.ParentFolderID,
		TotalMessages:  folder.TotalMessages,
		UnreadMessages: folder.UnreadMessages,
		Labels:         cloneMap(folder.Labels),
		UpdatedAt:      toProtoTime(folder.UpdatedAt),
	}
}

func toProtoFolders(folders []core.Folder) []*mailboxv1.MailFolder {
	out := make([]*mailboxv1.MailFolder, 0, len(folders))
	for _, folder := range folders {
		out = append(out, toProtoFolder(folder))
	}
	return out
}

func toProtoMessage(message core.Message) *mailboxv1.EmailMessage {
	return &mailboxv1.EmailMessage{
		Identity:    toProtoIdentity(message.Identity),
		From:        toProtoAddress(message.From),
		To:          toProtoAddresses(message.To),
		Cc:          toProtoAddresses(message.Cc),
		Bcc:         toProtoAddresses(message.Bcc),
		ReplyTo:     toProtoAddresses(message.ReplyTo),
		Subject:     message.Subject,
		SentAt:      toProtoTime(message.SentAt),
		ReceivedAt:  toProtoTime(message.ReceivedAt),
		Headers:     toProtoHeaders(message.Headers),
		TextBody:    message.TextBody,
		HtmlBody:    message.HTMLBody,
		Snippet:     message.Snippet,
		Flags:       toProtoFlags(message.Flags),
		Attachments: toProtoAttachments(message.Attachments),
		SizeBytes:   message.SizeBytes,
		Labels:      cloneMap(message.Labels),
	}
}

func toProtoSummary(summary core.MessageSummary) *mailboxv1.EmailMessageSummary {
	return &mailboxv1.EmailMessageSummary{
		Identity:       toProtoIdentity(summary.Identity),
		From:           toProtoAddress(summary.From),
		To:             toProtoAddresses(summary.To),
		Subject:        summary.Subject,
		Snippet:        summary.Snippet,
		SentAt:         toProtoTime(summary.SentAt),
		ReceivedAt:     toProtoTime(summary.ReceivedAt),
		Flags:          toProtoFlags(summary.Flags),
		HasAttachments: summary.HasAttachments,
		SizeBytes:      summary.SizeBytes,
		Labels:         cloneMap(summary.Labels),
	}
}

func toProtoSummaries(summaries []core.MessageSummary) []*mailboxv1.EmailMessageSummary {
	out := make([]*mailboxv1.EmailMessageSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, toProtoSummary(summary))
	}
	return out
}

func toProtoIdentity(identity core.MessageIdentity) *mailboxv1.MessageIdentity {
	return &mailboxv1.MessageIdentity{
		EmailId:         identity.EmailID,
		AccountId:       identity.AccountID,
		FolderId:        identity.FolderID,
		ThreadId:        identity.ThreadID,
		RfcMessageId:    identity.RFCMessageID,
		SourceMessageId: identity.SourceMessageID,
		Uid:             identity.UID,
		UidValidity:     identity.UIDValidity,
	}
}

func toProtoAddress(address core.Address) *mailboxv1.MailboxAddress {
	if address.Email == "" && address.DisplayName == "" {
		return nil
	}
	return &mailboxv1.MailboxAddress{Email: address.Email, DisplayName: address.DisplayName}
}

func toProtoAddresses(addresses []core.Address) []*mailboxv1.MailboxAddress {
	out := make([]*mailboxv1.MailboxAddress, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, toProtoAddress(address))
	}
	return out
}

func toProtoHeaders(headers []core.Header) []*mailboxv1.MailHeader {
	out := make([]*mailboxv1.MailHeader, 0, len(headers))
	for _, header := range headers {
		out = append(out, &mailboxv1.MailHeader{Name: header.Name, Value: header.Value})
	}
	return out
}

func toProtoAttachments(attachments []core.Attachment) []*mailboxv1.MailAttachment {
	out := make([]*mailboxv1.MailAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, &mailboxv1.MailAttachment{
			AttachmentId: attachment.ID,
			Filename:     attachment.Filename,
			ContentType:  attachment.ContentType,
			SizeBytes:    attachment.SizeBytes,
			ContentId:    attachment.ContentID,
			Inline:       attachment.Inline,
			Disposition:  attachment.Disposition,
		})
	}
	return out
}

func fromProtoCriteria(criteria *mailboxv1.MessageSearchCriteria) core.SearchCriteria {
	if criteria == nil {
		return core.SearchCriteria{}
	}
	return core.SearchCriteria{
		AccountID:           criteria.GetAccountId(),
		FolderID:            criteria.GetFolderId(),
		FromEmail:           criteria.GetFromEmail(),
		ToEmail:             criteria.GetToEmail(),
		SubjectContains:     criteria.GetSubjectContains(),
		BodyContains:        criteria.GetBodyContains(),
		HeaderName:          criteria.GetHeaderName(),
		HeaderValueContains: criteria.GetHeaderValueContains(),
		ReceivedAfter:       protoTime(criteria.GetReceivedAfter()),
		ReceivedBefore:      protoTime(criteria.GetReceivedBefore()),
		RequiredFlags:       fromProtoFlags(criteria.GetRequiredFlags()),
		IncludeDeleted:      criteria.GetIncludeDeleted(),
		Query:               criteria.GetQuery(),
	}
}

func fromProtoFlags(flags []mailboxv1.MailFlag) []core.Flag {
	out := make([]core.Flag, 0, len(flags))
	for _, flag := range flags {
		switch flag {
		case mailboxv1.MailFlag_MAIL_FLAG_SEEN:
			out = append(out, core.FlagSeen)
		case mailboxv1.MailFlag_MAIL_FLAG_ANSWERED:
			out = append(out, core.FlagAnswered)
		case mailboxv1.MailFlag_MAIL_FLAG_FLAGGED:
			out = append(out, core.FlagFlagged)
		case mailboxv1.MailFlag_MAIL_FLAG_DELETED:
			out = append(out, core.FlagDeleted)
		case mailboxv1.MailFlag_MAIL_FLAG_DRAFT:
			out = append(out, core.FlagDraft)
		case mailboxv1.MailFlag_MAIL_FLAG_RECENT:
			out = append(out, core.FlagRecent)
		case mailboxv1.MailFlag_MAIL_FLAG_IMPORTANT:
			out = append(out, core.FlagImportant)
		}
	}
	return out
}

func toProtoFlags(flags []core.Flag) []mailboxv1.MailFlag {
	out := make([]mailboxv1.MailFlag, 0, len(flags))
	for _, flag := range flags {
		switch flag {
		case core.FlagSeen:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_SEEN)
		case core.FlagAnswered:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_ANSWERED)
		case core.FlagFlagged:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_FLAGGED)
		case core.FlagDeleted:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_DELETED)
		case core.FlagDraft:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_DRAFT)
		case core.FlagRecent:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_RECENT)
		case core.FlagImportant:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_IMPORTANT)
		default:
			out = append(out, mailboxv1.MailFlag_MAIL_FLAG_UNSPECIFIED)
		}
	}
	return out
}

func toProtoAccountStatus(status core.AccountStatus) mailboxv1.EmailAccountStatus {
	switch status {
	case core.AccountStatusActive:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_ACTIVE
	case core.AccountStatusSuspended:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_SUSPENDED
	case core.AccountStatusDisabled:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_DISABLED
	case core.AccountStatusExpired:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_EXPIRED
	case core.AccountStatusFailed:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_FAILED
	default:
		return mailboxv1.EmailAccountStatus_EMAIL_ACCOUNT_STATUS_UNSPECIFIED
	}
}

func toProtoFolderRole(role core.FolderRole) mailboxv1.MailFolderRole {
	switch role {
	case core.FolderRoleInbox:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_INBOX
	case core.FolderRoleArchive:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_ARCHIVE
	case core.FolderRoleSent:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_SENT
	case core.FolderRoleDrafts:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_DRAFTS
	case core.FolderRoleTrash:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_TRASH
	case core.FolderRoleJunk:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_JUNK
	case core.FolderRoleCustom:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_CUSTOM
	default:
		return mailboxv1.MailFolderRole_MAIL_FOLDER_ROLE_UNSPECIFIED
	}
}

func toProtoError(err error) *mailboxv1.MailboxError {
	if err == nil {
		return nil
	}
	var mailboxErr *core.Error
	if !errors.As(err, &mailboxErr) {
		mailboxErr = core.NewError(core.CodeInternal, err.Error(), false)
	}
	return &mailboxv1.MailboxError{
		Code:      toProtoErrorCode(mailboxErr.Code),
		Message:   mailboxErr.Message,
		Retryable: mailboxErr.Retryable,
	}
}

func toProtoErrorCode(code core.ErrorCode) mailboxv1.MailboxErrorCode {
	switch code {
	case core.CodeValidationFailed:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_VALIDATION_FAILED
	case core.CodeAccountNotFound:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_ACCOUNT_NOT_FOUND
	case core.CodeFolderNotFound:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_FOLDER_NOT_FOUND
	case core.CodeMessageNotFound:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_MESSAGE_NOT_FOUND
	case core.CodeAuthenticationFailed:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_AUTHENTICATION_FAILED
	case core.CodePermissionDenied:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_PERMISSION_DENIED
	case core.CodeRateLimited:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_RATE_LIMITED
	case core.CodeProviderUnavailable:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_PROVIDER_UNAVAILABLE
	case core.CodeTimeout:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_TIMEOUT
	case core.CodeUnsupportedOperation:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_UNSUPPORTED_OPERATION
	case core.CodeMessageExpired:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_MESSAGE_EXPIRED
	case core.CodeInternal:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_INTERNAL
	default:
		return mailboxv1.MailboxErrorCode_MAILBOX_ERROR_CODE_UNSPECIFIED
	}
}

func protoDuration(value *durationpb.Duration) time.Duration {
	if value == nil {
		return 0
	}
	return value.AsDuration()
}

func protoTime(value *timestamppb.Timestamp) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.AsTime()
}

func toProtoTime(value time.Time) *timestamppb.Timestamp {
	if value.IsZero() {
		return nil
	}
	return timestamppb.New(value)
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
