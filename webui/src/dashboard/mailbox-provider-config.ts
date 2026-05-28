import type { Mailbox } from './types';

export type MailboxProviderTab = 'outlook' | 'cloudflare';
export type MailboxCredentialField = 'password' | 'refresh_token' | 'access_token';

export type MailboxProviderUIConfig = {
  value: MailboxProviderTab;
  label: string;
  providerKey: string;
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
  providerKey: 'outlook',
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
  providerKey: 'cloudflare',
  aliases: ['cf'],
  showStatus: false,
  tokenText: 'Webhook',
}] satisfies MailboxProviderUIConfig[];

export function mailboxProviderConfig(provider: string): MailboxProviderUIConfig {
  const value = mailboxProviderValue(provider);
  return mailboxProviderConfigs.find((item) => item.value === value) || mailboxProviderConfigs[0];
}

export function mailboxProviderValue(provider: string): MailboxProviderTab {
  return mailboxProviderTabFor(provider) || 'outlook';
}

export function mailboxProviderMatches(provider: string, target: MailboxProviderTab) {
  return mailboxProviderTabFor(provider) === target;
}

export function mailboxProviderTabFor(provider: string): MailboxProviderTab | undefined {
  return mailboxProviderConfigFor(provider)?.value;
}

function mailboxProviderConfigFor(provider: string) {
  const normalized = String(provider || '').trim().toLowerCase();
  return mailboxProviderConfigs.find((item) => (
    normalized === item.value ||
    String(item.label).toLowerCase() === normalized ||
    item.providerKey === normalized ||
    item.aliases.includes(normalized)
  ));
}
