import { useState } from 'react';
import type { ComponentType } from 'react';
import {
  PanelTabs,
  ToolbarActionButtons
} from '@byte-v-forge/common-ui';
import { CloudflareMailboxProviderPanel } from './mailbox-provider-cloudflare';
import { MailboxImportSheet } from './mailbox-import';
import { OutlookMailboxProviderPanel } from './mailbox-provider-outlook';
import type { MailboxProviderPanelProps } from './mailbox-provider-types';
import { providerToolbarActions } from './mailbox-toolbar-actions';
import { capabilityForProvider, mailboxProviderConfigs, mailboxProviderMatches, type MailboxProviderTab } from './mailbox-utils';
import type { Mailbox, MailboxDomain, MailboxOperation, MailboxProviderCapability } from './types';
export { MailboxDetails } from './mailbox-details';

const providerComponents: Record<MailboxProviderTab, ComponentType<MailboxProviderPanelProps>> = {
  outlook: OutlookMailboxProviderPanel,
  cloudflare: CloudflareMailboxProviderPanel,
};

const providerDefinitions = mailboxProviderConfigs.map((config) => ({
  value: config.value,
  label: config.label,
  Component: providerComponents[config.value],
})) satisfies ProviderDefinition[];

export function MailboxPanel(props: MailboxPanelProps) {
  const [activeProvider, setActiveProvider] = useState<MailboxProviderTab>('outlook');
  const [importProvider, setImportProvider] = useState<MailboxProviderTab>();
  const panelProps = providerPanelProps(props);
  const providerViews = providerDefinitions.map((definition) => ({
    ...definition,
    capability: capabilityForProvider(props.providerCapabilities, definition.value),
    mailboxes: props.mailboxes.filter((mailbox) => mailboxProviderMatches(mailbox.provider_key, definition.value)),
  }));
  const activeView = providerViews.find((view) => view.value === activeProvider) || providerViews[0];
  const toolbarActions = providerToolbarActions(activeView, props, setImportProvider);

  return (
    <>
      <PanelTabs
        value={activeProvider}
        onValueChange={(value) => setActiveProvider(value as MailboxProviderTab)}
        tabsClassName="min-h-0 flex-1 overflow-hidden"
        tabsListVariant="line"
        tabsListClassName="h-8"
        actions={<ToolbarActionButtons actions={toolbarActions} />}
        tabs={providerViews.map(({ value, label, capability, mailboxes, Component }) => ({
          value,
          label: capability?.display_name || label,
          triggerClassName: 'gap-1.5 px-2',
          contentClassName: 'overflow-auto',
          content: <Component {...panelProps} mailboxes={mailboxes} capability={capability} />
        }))}
      />
      <MailboxImportSheet open={!!importProvider} provider={importProvider || activeProvider} busy={props.busy} onOpenChange={(open) => !open && setImportProvider(undefined)} onDone={props.onDone} onError={props.onError} />
    </>
  );
}

type ProviderDefinition = {
  value: MailboxProviderTab;
  label: string;
  Component: ComponentType<MailboxProviderPanelProps>;
};

type ProviderView = {
  value: MailboxProviderTab;
  label: string;
  capability?: MailboxProviderCapability;
  mailboxes: Mailbox[];
  Component: ComponentType<MailboxProviderPanelProps>;
};

type MailboxPanelProps = {
  mailboxes: Mailbox[];
  domains: MailboxDomain[];
  providerCapabilities: MailboxProviderCapability[];
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

function providerPanelProps(props: MailboxPanelProps): Omit<MailboxProviderPanelProps, 'mailboxes' | 'capability'> {
  return {
    domains: props.domains,
    selected: props.selected,
    busy: props.busy,
    showSecrets: props.showSecrets,
    oauthing: props.oauthing,
    inboxLoading: props.inboxLoading,
    domainSyncing: props.domainSyncing,
    runningOperationByEmail: props.runningOperationByEmail,
    onSelect: props.onSelect,
    onOAuth: props.onOAuth,
    onFetchInbox: props.onFetchInbox,
    onSyncDomains: props.onSyncDomains,
    onToggleSecrets: props.onToggleSecrets,
    onDelete: props.onDelete,
    onDone: props.onDone,
    onError: props.onError
  };
}
