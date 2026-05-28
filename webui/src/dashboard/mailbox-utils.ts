import { MailboxCredentialKind, MailboxProviderAction } from '@byte-v-forge/common-ui';
import { normalizeUiEmail } from './email-utils';
import { mailboxProviderConfig, mailboxProviderConfigs, mailboxProviderMatches, mailboxProviderTabFor, mailboxProviderValue, type MailboxProviderTab } from './mailbox-provider-config';
import type { Mailbox, MailboxProviderActionCapability, MailboxProviderCapability } from './types';

export { mailboxProviderConfig, mailboxProviderConfigs, mailboxProviderMatches, mailboxProviderTabFor, mailboxProviderValue, type MailboxProviderTab };
export type MailboxActionKey = 'import_mailbox' | 'run_oauth' | 'fetch_inbox' | 'receive_webhook' | 'auto_create_mailbox' | 'sync_domains';

export type MailboxBatchItem = {
  email: string;
  password: string;
};

export const mailboxActions = {
  importMailbox: 'import_mailbox',
  runOAuth: 'run_oauth',
  fetchInbox: 'fetch_inbox',
  receiveWebhook: 'receive_webhook',
  autoCreateMailbox: 'auto_create_mailbox',
  syncDomains: 'sync_domains'
} as const;

const mailboxActionByProto: Partial<Record<MailboxProviderAction, MailboxActionKey>> = {
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_IMPORT_MAILBOX]: mailboxActions.importMailbox,
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_RUN_OAUTH]: mailboxActions.runOAuth,
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_FETCH_INBOX]: mailboxActions.fetchInbox,
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_RECEIVE_WEBHOOK]: mailboxActions.receiveWebhook,
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_AUTO_CREATE_MAILBOX]: mailboxActions.autoCreateMailbox,
  [MailboxProviderAction.MAILBOX_PROVIDER_ACTION_SYNC_DOMAINS]: mailboxActions.syncDomains
};

export function domainForEmail(email: string) {
  const [, domain = ''] = normalizeUiEmail(email).split('@');
  return domain;
}

export function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.map((value) => value.trim()).filter(Boolean))).sort();
}

export function tokenText(mailbox: Mailbox) {
  const configured = mailboxProviderConfig(mailbox.provider_key).tokenText;
  if (typeof configured === 'function') return configured(mailbox);
  if (configured) return configured;
  if (mailbox.refresh_token && authStatus(mailbox) === 'AUTHORIZED') return 'Refresh 可用';
  if (mailbox.refresh_token) return 'Refresh 待验证';
  if (mailbox.access_token) return '仅 Access';
  return '缺 Token';
}

export function mailboxProviderText(provider: string) {
  return mailboxProviderConfig(provider).label;
}

export function authStatus(mailbox: Mailbox) {
  const value = String(mailbox.auth_status || '').trim();
  if (value) return value;
  if (mailbox.refresh_token) return 'AUTHORIZED';
  return 'OAUTH_PENDING';
}

export function parseMailboxBatch(value: string, provider: string) {
  const items: MailboxBatchItem[] = [];
  const errors: string[] = [];
  const importConfig = mailboxProviderConfig(provider).import;
  if (!importConfig) {
    return { items, errors: ['当前 provider 不支持导入'] };
  }
  const allowPlainEmailBatch = importConfig.allowPlainEmailBatch;

  value.split(/\r?\n/).forEach((raw, index) => {
    const line = raw.trim();
    if (!line) return;
    const delimiterIndex = line.indexOf('----');
    if (allowPlainEmailBatch && delimiterIndex < 0) {
      items.push({ email: line, password: '' });
      return;
    }
    if (delimiterIndex < 0) {
      errors.push(`第 ${index + 1} 行缺少 ----`);
      return;
    }
    const email = line.slice(0, delimiterIndex).trim();
    const password = line.slice(delimiterIndex + 4).trim();
    if (!email) {
      errors.push(`第 ${index + 1} 行缺少账号`);
      return;
    }
    items.push({ email, password });
  });

  return { items, errors };
}

export function capabilityForProvider(capabilities: MailboxProviderCapability[], provider: string) {
  const target = mailboxProviderTabFor(provider);
  if (!target) return undefined;
  return capabilities.find((capability) => mailboxProviderMatches(capability.key, target));
}

export function providerAction(capability: MailboxProviderCapability | undefined, action: MailboxActionKey) {
  return (capability?.actions || []).find((item) => actionKey(item.action) === action);
}

export function canRunMailboxAction(mailbox: Mailbox, action: MailboxProviderActionCapability | undefined) {
  if (!action) return false;
  if (!requiredCredentialsPresent(mailbox, action.required_credentials || [])) return false;
  const statuses = action.required_auth_statuses || [];
  return statuses.length === 0 || statuses.includes(authStatus(mailbox));
}

export function bulkMailboxActionCount(mailboxes: Mailbox[], action: MailboxProviderActionCapability | undefined) {
  if (!action?.bulk_supported) return 0;
  return mailboxes.filter((mailbox) => canRunMailboxAction(mailbox, action)).length;
}

export function canRunProviderMailboxAction(capabilities: MailboxProviderCapability[], mailbox: Mailbox, action: MailboxActionKey) {
  return canRunMailboxAction(mailbox, providerAction(capabilityForProvider(capabilities, mailbox.provider_key), action));
}

export function actionKey(action: MailboxProviderAction): MailboxActionKey | '' {
  return mailboxActionByProto[action] || '';
}

function requiredCredentialsPresent(mailbox: Mailbox, credentials: MailboxCredentialKind[]) {
  return credentials.every((credential) => credentialPresent(mailbox, credential));
}

function credentialPresent(mailbox: Mailbox, credential: MailboxCredentialKind) {
  switch (credential) {
    case MailboxCredentialKind.MAILBOX_CREDENTIAL_KIND_UNSPECIFIED:
      return true;
    case MailboxCredentialKind.MAILBOX_CREDENTIAL_KIND_PASSWORD:
      return !!String(mailbox.password || '').trim();
    case MailboxCredentialKind.MAILBOX_CREDENTIAL_KIND_OAUTH_REFRESH_TOKEN:
      return !!String(mailbox.refresh_token || '').trim();
    case MailboxCredentialKind.MAILBOX_CREDENTIAL_KIND_OAUTH_ACCESS_TOKEN:
      return !!String(mailbox.access_token || '').trim();
    default:
      return false;
  }
}
