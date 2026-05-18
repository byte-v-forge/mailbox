package grpcadapter

import (
	"context"

	mailboxv1 "github.com/byte-v-forge/contracts-go/byte/v/forge/contracts/mailbox/v1"
	"github.com/byte-v-forge/mailbox/internal/app"
)

type MessageServer struct {
	mailboxv1.UnimplementedMailboxMessageServiceServer
	service *app.MailboxService
}

func NewMessageServer(service *app.MailboxService) *MessageServer {
	return &MessageServer{service: service}
}

func (s *MessageServer) SearchMessages(ctx context.Context, request *mailboxv1.SearchMessagesRequest) (*mailboxv1.SearchMessagesResponse, error) {
	result, err := s.service.SearchMessages(ctx, fromProtoCriteria(request.GetCriteria()), int(request.GetPageSize()), request.GetPageToken())
	if err != nil {
		return &mailboxv1.SearchMessagesResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.SearchMessagesResponse{
		Messages:      toProtoSummaries(result.Messages),
		NextPageToken: result.NextPageToken,
	}, nil
}

func (s *MessageServer) GetMessage(ctx context.Context, request *mailboxv1.GetMessageRequest) (*mailboxv1.GetMessageResponse, error) {
	message, err := s.service.GetMessage(ctx, request.GetAccountId(), request.GetEmailId(), app.GetMessageOptions{
		IncludeHeaders:     request.GetIncludeHeaders(),
		IncludeBody:        request.GetIncludeBody(),
		IncludeAttachments: request.GetIncludeAttachments(),
	})
	if err != nil {
		return &mailboxv1.GetMessageResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.GetMessageResponse{Message: toProtoMessage(message)}, nil
}

func (s *MessageServer) WaitForMessage(ctx context.Context, request *mailboxv1.WaitForMessageRequest) (*mailboxv1.WaitForMessageResponse, error) {
	message, err := s.service.WaitForMessage(ctx, fromProtoCriteria(request.GetCriteria()), protoDuration(request.GetTimeout()), protoDuration(request.GetPollInterval()))
	if err != nil {
		return &mailboxv1.WaitForMessageResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.WaitForMessageResponse{Message: toProtoMessage(message)}, nil
}

func (s *MessageServer) AddMessageFlags(ctx context.Context, request *mailboxv1.AddMessageFlagsRequest) (*mailboxv1.AddMessageFlagsResponse, error) {
	summary, err := s.service.AddMessageFlags(ctx, request.GetAccountId(), request.GetEmailId(), fromProtoFlags(request.GetFlags()))
	if err != nil {
		return &mailboxv1.AddMessageFlagsResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.AddMessageFlagsResponse{Message: toProtoSummary(summary)}, nil
}

func (s *MessageServer) RemoveMessageFlags(ctx context.Context, request *mailboxv1.RemoveMessageFlagsRequest) (*mailboxv1.RemoveMessageFlagsResponse, error) {
	summary, err := s.service.RemoveMessageFlags(ctx, request.GetAccountId(), request.GetEmailId(), fromProtoFlags(request.GetFlags()))
	if err != nil {
		return &mailboxv1.RemoveMessageFlagsResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.RemoveMessageFlagsResponse{Message: toProtoSummary(summary)}, nil
}

func (s *MessageServer) MoveMessage(ctx context.Context, request *mailboxv1.MoveMessageRequest) (*mailboxv1.MoveMessageResponse, error) {
	summary, err := s.service.MoveMessage(ctx, request.GetAccountId(), request.GetEmailId(), request.GetTargetFolderId())
	if err != nil {
		return &mailboxv1.MoveMessageResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.MoveMessageResponse{Message: toProtoSummary(summary)}, nil
}

func (s *MessageServer) DeleteMessage(ctx context.Context, request *mailboxv1.DeleteMessageRequest) (*mailboxv1.DeleteMessageResponse, error) {
	if err := s.service.DeleteMessage(ctx, request.GetAccountId(), request.GetEmailId(), request.GetPermanent()); err != nil {
		return &mailboxv1.DeleteMessageResponse{Error: toProtoError(err)}, nil
	}
	return &mailboxv1.DeleteMessageResponse{}, nil
}
