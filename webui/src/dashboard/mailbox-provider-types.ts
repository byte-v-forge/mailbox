import type { Mailbox, MailboxDomain, MailboxOperation, MailboxProviderCapability } from './types';

export type MailboxProviderPanelProps = {
  mailboxes: Mailbox[];
  domains: MailboxDomain[];
  capability?: MailboxProviderCapability;
  selected?: string;
  busy: boolean;
  showSecrets: boolean;
  oauthing: string;
  inboxLoading: boolean;
  domainSyncing: boolean;
  runningOperationByEmail: Map<string, MailboxOperation>;
  onSelect: (mailbox: Mailbox) => void;
  onOAuth: (emailAddress?: string) => Promise<void>;
  onFetchInbox: () => Promise<void>;
  onSyncDomains: (providerKey: string) => Promise<void>;
  onToggleSecrets: () => void;
  onDelete: (mailbox: Mailbox) => Promise<void>;
  onDone: (message: string) => void;
  onError: (message: string) => void;
};
