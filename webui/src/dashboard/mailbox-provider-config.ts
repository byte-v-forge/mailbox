import { MailboxProvider } from '@/proto/mailbox_service';
import type { Mailbox } from './types';

export type MailboxProviderTab = 'outlook' | 'cloudflare';
export type MailboxCredentialField = 'password' | 'refresh_token' | 'access_token';

export type MailboxProviderUIConfig = {
  value: MailboxProviderTab;
  label: string;
  provider: MailboxProvider;
  aliases: string[];
  showStatus: boolean;
  tokenText?: string | ((mailbox: Mailbox) => string);
  import?: {
    description: string;
    batchPlaceholder: string;
    allowPlainEmailBatch: boolean;
    credentialFields: MailboxCredentialField[];
  };
};

export const mailboxProviderConfigs = [{
  value: 'outlook',
  label: 'Outlook',
  provider: MailboxProvider.MAILBOX_PROVIDER_OUTLOOK,
  aliases: ['microsoft', 'graph'],
  showStatus: true,
  import: {
    description: 'Outlook 可附带密码或 OAuth token。',
    batchPlaceholder: 'account@example.com----password',
    allowPlainEmailBatch: false,
    credentialFields: ['password', 'refresh_token', 'access_token'],
  },
}, {
  value: 'cloudflare',
  label: 'Cloudflare',
  provider: MailboxProvider.MAILBOX_PROVIDER_CLOUDFLARE,
  aliases: ['cf'],
  showStatus: false,
  tokenText: 'Webhook',
}] satisfies MailboxProviderUIConfig[];

export function mailboxProviderConfig(provider: string): MailboxProviderUIConfig {
  const value = mailboxProviderValue(provider);
  return mailboxProviderConfigs.find((item) => item.value === value) || mailboxProviderConfigs[0];
}

export function mailboxProviderValue(provider: string): MailboxProviderTab {
  const normalized = String(provider || '').trim().toLowerCase();
  const matched = mailboxProviderConfigs.find((item) => (
    normalized === item.value ||
    String(item.label).toLowerCase() === normalized ||
    item.provider.toLowerCase() === normalized ||
    item.aliases.includes(normalized)
  ));
  if (matched) return matched.value;
  return 'outlook';
}
