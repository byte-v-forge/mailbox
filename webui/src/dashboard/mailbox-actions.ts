import { useEffect, useMemo, useState } from 'react';
import {
  api,
  short,
  type FetchMailboxInboxesRequest,
  type ListMailboxInboxResponse,
  type StartMailboxOAuthRequest,
  type StartMailboxOAuthResponse,
  type SyncMailboxDomainsRequest,
  type SyncMailboxDomainsResponse,
  useQuery,
  useQueryClient,
  useToastMessage
} from '@byte-v-forge/common-ui';
import { maskEmail, normalizeUiEmail } from './email-utils';
import type { MailboxData } from './mailbox-data';
import type { DeleteMailboxResponse } from '../proto/email';
import type { InboxResponse, InboxResult, Mailbox } from './types';

export const mailboxInboxQueryKey = (email: string) => ['mailbox', 'inbox', normalizeUiEmail(email)] as const;

export function useMailboxActions(data: MailboxData, showSecrets: boolean, setSelectedEmail: (value: string | ((prev: string) => string)) => void) {
  const toast = useToastMessage();
  const queryClient = useQueryClient();
  const selectedEmail = normalizeUiEmail(data.selected?.email_address || '');
  const selectedInboxKey = useMemo(() => mailboxInboxQueryKey(selectedEmail), [selectedEmail]);
  const inboxQuery = useQuery<InboxResult | null>({
    queryKey: selectedInboxKey,
    queryFn: () => fetchStoredInbox(selectedEmail),
    enabled: false,
    initialData: null
  });
  const [oauthing, setOAuthing] = useState('');
  const [inboxLoading, setInboxLoading] = useState(false);
  const [storedInboxLoading, setStoredInboxLoading] = useState(false);
  const [domainSyncing, setDomainSyncing] = useState(false);

  useEffect(() => { if (data.loadError) toast.showError(data.loadError); }, [data.loadError, toast.showError]);

  useEffect(() => {
    if (!selectedEmail) return;
    let cancelled = false;
    setStoredInboxLoading(true);
    fetchStoredInbox(selectedEmail)
      .then((result) => {
        if (!cancelled) queryClient.setQueryData(selectedInboxKey, result);
      })
      .catch((err) => {
        if (!cancelled) toast.showError(err);
      })
      .finally(() => {
        if (!cancelled) setStoredInboxLoading(false);
      });
    return () => { cancelled = true; };
  }, [selectedEmail, selectedInboxKey, queryClient, toast.showError]);

  async function runOAuth(emailAddress = '') {
    setOAuthing(emailAddress || '*');
    try {
      const input: StartMailboxOAuthRequest = { email_address: emailAddress, only_missing: !emailAddress, limit: 100 };
      const resp = await api<StartMailboxOAuthResponse>('/api/mailbox/mailboxes/oauth', { method: 'POST', body: JSON.stringify(input) });
      toast.showToast(!resp.started || resp.error_message ? 'error' : 'ok', resp.error_message || (!resp.started ? 'OAuth 流程启动失败' : `OAuth 流程已提交: ${short(resp.operation_id)}`));
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setOAuthing('');
    }
  }

  async function fetchInbox(emailAddress = '') {
    setInboxLoading(true);
    try {
      const target = emailAddress.trim();
      const input: FetchMailboxInboxesRequest = { limit_per_mailbox: 10, max_mailboxes: target ? 1 : 200, email_address: target, parser_profile: '', received_after_unix: 0 };
      const resp = await api<InboxResponse>('/api/mailbox/mailboxes/inbox', { method: 'POST', body: JSON.stringify(input) });
      for (const result of resp.results || []) {
        const email = result.mailbox?.email_address || result.messages?.[0]?.mailbox_email || target;
        if (email) queryClient.setQueryData(mailboxInboxQueryKey(email), result);
      }
      toast.showToast(resp.failed_count > 0 ? 'error' : 'ok', `${target ? `${showSecrets ? target : maskEmail(target)} ` : ''}收信完成：${resp.message_count} 封邮件`);
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setInboxLoading(false);
    }
  }

  async function syncProviderDomains(providerKey: string) {
    const targetProvider = providerKey.trim();
    if (!targetProvider) {
      toast.showError('provider_key is required');
      return;
    }
    setDomainSyncing(true);
    try {
      const input: SyncMailboxDomainsRequest = { provider_key: targetProvider };
      const resp = await api<SyncMailboxDomainsResponse>('/api/mailbox/domains', {
        method: 'POST',
        body: JSON.stringify(input)
      });
      toast.showToast(resp.error_message ? 'error' : 'ok', resp.error_message || `${providerDisplayName(data, targetProvider)} 域名已同步: ${resp.synced_count || 0}`);
      await data.invalidate();
    } catch (err) {
      toast.showError(err);
    } finally {
      setDomainSyncing(false);
    }
  }

  async function deleteMailbox(mailbox: Mailbox) {
    if (!window.confirm(`删除邮箱 ${showSecrets ? mailbox.email_address : maskEmail(mailbox.email_address)}？`)) return;
    await api<DeleteMailboxResponse>(`/api/mailbox/mailboxes/${encodeURIComponent(mailbox.email_address)}`, { method: 'DELETE' });
    setSelectedEmail((prev) => prev === mailbox.email_address ? '' : prev);
    toast.showOK('邮箱已删除');
    await data.invalidate();
  }

  async function done(message: string) {
    toast.showOK(message);
    await data.invalidate();
  }

  return { toast, inboxResult: inboxQuery.data ?? null, inboxQueryKey: selectedInboxKey, oauthing, inboxLoading: storedInboxLoading || inboxQuery.isFetching || inboxLoading, domainSyncing, runOAuth, fetchInbox, syncProviderDomains, deleteMailbox, done };
}

async function fetchStoredInbox(email: string) {
  const resp = await api<ListMailboxInboxResponse>(`/api/mailbox/mailboxes/${encodeURIComponent(email)}/inbox?limit=20`);
  return resp.result || null;
}

function providerDisplayName(data: MailboxData, providerKey: string) {
  const normalized = providerKey.trim().toLowerCase();
  return data.providerCapabilities.find((item) => item.key.trim().toLowerCase() === normalized)?.display_name || providerKey;
}
