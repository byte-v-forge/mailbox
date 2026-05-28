import {
  mailboxEventURL as commonMailboxEventURL,
  mergeInboxMessage,
  useMailboxEmailEventCache as useCommonMailboxEmailEventCache
} from '@byte-v-forge/common-ui';
import type { MailboxEmailEventCacheOptions as CommonMailboxEmailEventCacheOptions } from '@byte-v-forge/common-ui';

const mailboxApiBase = '/api/mailbox';

export type MailboxEmailEventCacheOptions = Omit<CommonMailboxEmailEventCacheOptions, 'apiBase'>;
export { mergeInboxMessage };

export function useMailboxEmailEventCache(options: MailboxEmailEventCacheOptions) {
  useCommonMailboxEmailEventCache({ ...options, apiBase: mailboxApiBase });
}

export function mailboxEventURL(email: string, signalKind = 'otp', issuedAfterUnix = 0) {
  return commonMailboxEventURL(mailboxApiBase, email, signalKind, issuedAfterUnix);
}
