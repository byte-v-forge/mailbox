import type { EmailInboxMessage, EmailMailbox, EmailSignal, FetchMailboxInboxResult } from '@/proto/email';
import type {
  FetchMailboxInboxesResponse,
  MailboxDomain,
  MailboxProviderActionCapability,
  MailboxProviderCapabilities
} from '@/proto/mailbox_service';
import type { Job, JobSnapshot } from '@/dashboard/modules/workflow/sdk';

export type { EmailSignal, Job, JobSnapshot, MailboxDomain, MailboxProviderActionCapability };

export type MailboxProviderCapability = MailboxProviderCapabilities;

export type Mailbox = EmailMailbox;

export type InboxMessage = EmailInboxMessage & {
  otp?: string;
};

export type InboxResult = Omit<FetchMailboxInboxResult, 'mailbox' | 'messages'> & {
  mailbox?: Mailbox;
  messages?: InboxMessage[];
};

export type InboxResponse = Omit<FetchMailboxInboxesResponse, 'results' | 'operation_id'> & {
  operation_id?: string;
  results?: InboxResult[];
};

export type LatestOtp = {
  otp: string;
  subject: string;
  received_at_unix: number;
};

export type DisplayLabelMap = Record<string, string>;
