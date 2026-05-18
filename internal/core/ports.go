package core

import (
	"context"
	"time"
)

type Clock interface {
	Now() time.Time
}

type AccountStore interface {
	GetAccount(ctx context.Context, accountID string) (Account, error)
	ListFolders(ctx context.Context, accountID string) ([]Folder, error)
}

type MessageStore interface {
	SearchMessages(ctx context.Context, criteria SearchCriteria, pageSize int, pageToken string) (SearchResult, error)
	GetMessage(ctx context.Context, accountID, emailID string) (Message, error)
	SaveMessage(ctx context.Context, message Message) error
	UpdateMessage(ctx context.Context, message Message) error
	DeleteMessage(ctx context.Context, accountID, emailID string, permanent bool) error
}

type Provider interface {
	Key() string
	GetAccount(ctx context.Context, accountID string) (Account, error)
	ListFolders(ctx context.Context, accountID string) ([]Folder, error)
	SearchMessages(ctx context.Context, criteria SearchCriteria, pageSize int, pageToken string) (SearchResult, error)
	GetMessage(ctx context.Context, accountID, emailID string) (Message, error)
}
