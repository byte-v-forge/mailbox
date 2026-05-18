package grpcadapter

import (
	"context"

	mailboxv1 "github.com/byte-v-forge/contracts-go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/mailbox/internal/app"
)

type AccountServer struct {
	mailboxv1.UnimplementedMailboxAccountServiceServer
	service *app.MailboxService
}

func NewAccountServer(service *app.MailboxService) *AccountServer {
	return &AccountServer{service: service}
}

func (s *AccountServer) GetEmailAccount(ctx context.Context, request *mailboxv1.GetEmailAccountRequest) (*mailboxv1.GetEmailAccountResponse, error) {
	account, err := s.service.GetEmailAccount(ctx, request.GetAccountId())
	if err != nil {
		return &mailboxv1.GetEmailAccountResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.GetEmailAccountResponse{Account: toProtoAccount(account)}, nil
}

func (s *AccountServer) ListMailFolders(ctx context.Context, request *mailboxv1.ListMailFoldersRequest) (*mailboxv1.ListMailFoldersResponse, error) {
	folders, err := s.service.ListMailFolders(ctx, request.GetAccountId())
	if err != nil {
		return &mailboxv1.ListMailFoldersResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.ListMailFoldersResponse{Folders: toProtoFolders(folders)}, nil
}
