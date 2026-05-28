package main

import (
	mailboxv1 "github.com/byte-v-forge/common-lib/gen/go/byte/v/forge/contracts/mailbox/v1"

	"mailboxapi/pb"
)

func publicFetchInboxesResponse(resp *mailboxv1.FetchMailboxInboxesResponse, operationID string) *mailboxv1.FetchMailboxInboxesResponse {
	if resp == nil {
		return &mailboxv1.FetchMailboxInboxesResponse{OperationId: operationID}
	}
	return &mailboxv1.FetchMailboxInboxesResponse{
		Results:      append([]*mailboxv1.FetchMailboxInboxResult{}, resp.GetResults()...),
		MailboxCount: resp.GetMailboxCount(),
		FetchedCount: resp.GetFetchedCount(),
		FailedCount:  resp.GetFailedCount(),
		MessageCount: resp.GetMessageCount(),
		OperationId:  operationID,
	}
}

func publicMailbox(mailbox *pb.EmailMailbox) *mailboxv1.EmailMailbox {
	if mailbox == nil {
		return nil
	}
	return &mailboxv1.EmailMailbox{
		EmailAddress:    mailbox.GetEmailAddress(),
		LastError:       mailbox.GetLastError(),
		CreatedAt:       mailbox.GetCreatedAt(),
		UpdatedAt:       mailbox.GetUpdatedAt(),
		AuthStatus:      mailbox.GetAuthStatus(),
		ProviderKey:     mailbox.GetProviderKey(),
		LatestSignal:    mailbox.GetLatestSignal(),
		Domain:          mailbox.GetDomain(),
		CredentialState: publicMailboxCredentialState(mailbox),
	}
}

func publicMailboxCredentialState(mailbox *pb.EmailMailbox) *mailboxv1.MailboxCredentialState {
	if mailbox == nil {
		return nil
	}
	return &mailboxv1.MailboxCredentialState{
		PasswordPresent:          mailbox.GetPassword() != "",
		OauthRefreshTokenPresent: mailbox.GetRefreshToken() != "",
		OauthAccessTokenPresent:  mailbox.GetAccessToken() != "",
	}
}
