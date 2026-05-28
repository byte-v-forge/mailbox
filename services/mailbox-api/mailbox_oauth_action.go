package main

import (
	"context"

	"github.com/byte-v-forge/common-lib/emailx"

	"mailboxapi/pb"
)

func (a *mailboxActivities) runMailboxOAuthAction(ctx context.Context, operationID string, emailAddress string, onlyMissing bool, limit int32) mailboxOperationResult {
	selection, err := a.SelectMailboxOAuthAccounts(ctx, &pb.SelectMailboxOAuthAccountsRequest{
		OperationId:  operationID,
		EmailAddress: emailAddress,
		OnlyMissing:  onlyMissing,
		Limit:        limit,
	})
	if err != nil {
		return mailboxOperationResult{OperationID: operationID, Success: false, ErrorMessage: err.Error()}
	}

	accounts := selection.GetAccounts()
	results := make([]*pb.MailboxOAuthResult, 0, len(accounts))
	for _, account := range accounts {
		accountResult, err := a.RunMailboxOAuthAccount(ctx, &pb.RunMailboxOAuthAccountRequest{
			OperationId: operationID,
			Account:     account,
		})
		if err != nil {
			results = append(results, &pb.MailboxOAuthResult{
				EmailAddress: emailx.Normalize(account.GetEmailAddress()),
				Success:      false,
				ErrorMessage: err.Error(),
			})
			continue
		}
		if accountResult.GetResult() != nil {
			results = append(results, accountResult.GetResult())
		}
	}

	result, err := a.CompleteMailboxOAuth(ctx, &pb.CompleteMailboxOAuthRequest{
		OperationId: operationID,
		Accounts:    accounts,
		Results:     results,
	})
	if err != nil && result.ErrorMessage == "" {
		result.ErrorMessage = err.Error()
	}
	return result
}
