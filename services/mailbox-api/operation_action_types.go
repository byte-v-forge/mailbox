package main

type mailboxRegistrationActionInput struct {
	OperationID string
	ImportOnly  bool
}

type mailboxOperationResult struct {
	OperationID  string
	Success      bool
	ErrorMessage string
	ExitCode     int32
	MailboxCount int32
	FetchedCount int32
	FailedCount  int32
	MessageCount int32
}
