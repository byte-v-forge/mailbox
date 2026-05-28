import { useMemo } from 'react';
import {
  api,
  createHotStreamURL,
  type ListMailboxDomainsResponse,
  type ListMailboxOperationsResponse,
  type ListMailboxProviderCapabilitiesResponse,
  useHotStreamInvalidation,
  useQuery,
  useQueryClient
} from '@byte-v-forge/common-ui';
import { normalizeUiEmail } from './email-utils';
import type { ListEmailMailboxesResponse } from '../proto/email';
import type { Mailbox, MailboxOperation } from './types';

const mailboxQueryKeys = {
  mailboxes: ['mailbox', 'mailboxes'] as const,
  domains: ['mailbox', 'domains'] as const,
  providerCapabilities: ['mailbox', 'provider-capabilities'] as const,
  runningOperations: ['mailbox', 'running-operations'] as const
};

export function useMailboxData(selectedEmail: string) {
  const queryClient = useQueryClient();
  const mailboxesQuery = useQuery({ queryKey: mailboxQueryKeys.mailboxes, queryFn: () => api<ListEmailMailboxesResponse>('/api/mailbox/mailboxes?limit=500') });
  const domainsQuery = useQuery({ queryKey: mailboxQueryKeys.domains, queryFn: () => api<ListMailboxDomainsResponse>('/api/mailbox/domains') });
  const providerCapabilitiesQuery = useQuery({ queryKey: mailboxQueryKeys.providerCapabilities, queryFn: () => api<ListMailboxProviderCapabilitiesResponse>('/api/mailbox/provider-capabilities') });
  const runningOperationsQuery = useQuery({
    queryKey: mailboxQueryKeys.runningOperations,
    queryFn: () => api<ListMailboxOperationsResponse>('/api/mailbox/operations?limit=200&status=RUNNING')
  });
  useHotStreamInvalidation({
    url: createHotStreamURL('/api/mailbox', { eventTypes: ['mailbox.email.received', 'mailbox.email.signal_received', 'mailbox.operation.updated'] }),
    rules: [
      { queryKey: mailboxQueryKeys.mailboxes, eventTypes: ['mailbox.email.received', 'mailbox.email.signal_received'], resourceTypes: ['mailbox.email'] },
      { queryKey: mailboxQueryKeys.runningOperations, eventTypes: ['mailbox.operation.updated'], resourceTypes: ['mailbox.operation'] }
    ]
  });
  const mailboxes = Array.isArray(mailboxesQuery.data?.mailboxes) ? mailboxesQuery.data.mailboxes : [];
  const runningOperations = Array.isArray(runningOperationsQuery.data?.operations) ? runningOperationsQuery.data.operations : [];
  const selected = mailboxes.find((mailbox) => mailbox.email_address === selectedEmail) || null;
  const runningOperationByEmail = useMemo(() => latestOperationByEmail(runningOperations), [runningOperations]);

  return {
    mailboxes,
    selected,
    runningOperationByEmail,
    domains: Array.isArray(domainsQuery.data?.domains) ? domainsQuery.data.domains : [],
    providerCapabilities: Array.isArray(providerCapabilitiesQuery.data?.providers) ? providerCapabilitiesQuery.data.providers : [],
    busy: mailboxesQuery.isLoading || domainsQuery.isLoading || providerCapabilitiesQuery.isLoading,
    loadError: mailboxesQuery.error || domainsQuery.error || providerCapabilitiesQuery.error || runningOperationsQuery.error,
    invalidate: () => invalidateMailboxQueries(queryClient)
  };
}

export type MailboxData = ReturnType<typeof useMailboxData>;

function invalidateMailboxQueries(queryClient: ReturnType<typeof useQueryClient>) {
  return Promise.all(Object.values(mailboxQueryKeys).map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}

function latestOperationByEmail(operations: MailboxOperation[]) {
  const out = new Map<string, MailboxOperation>();
  for (const operation of operations) {
    const email = normalizeUiEmail(operation.email_address);
    if (!email) continue;
    const previous = out.get(email);
    if (!previous || (operation.updated_at || 0) > (previous.updated_at || 0)) out.set(email, operation);
  }
  return out;
}
