import type { EmailMailbox } from '../proto/email';
import type {
  EmailMailbox as InboxMailbox,
  EmailInboxMessage,
  EmailSignal,
  FetchMailboxInboxResult,
  FetchMailboxInboxesResponse,
  MailboxDomain,
  MailboxOperation,
  MailboxProviderActionCapability,
  MailboxProviderCapabilities
} from '@byte-v-forge/common-ui';

export type { EmailSignal, InboxMailbox, MailboxDomain, MailboxOperation, MailboxProviderActionCapability };

export type MailboxProviderCapability = MailboxProviderCapabilities;

export type Mailbox = EmailMailbox;

export type InboxMessage = EmailInboxMessage & {
  otp?: string;
};

export type InboxResult = Omit<FetchMailboxInboxResult, 'mailbox' | 'messages'> & {
  mailbox?: InboxMailbox;
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
