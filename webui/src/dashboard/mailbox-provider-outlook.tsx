import { MailboxRecordList } from './mailbox-list';
import type { MailboxProviderPanelProps } from './mailbox-provider-types';
import { mailboxProviderConfig } from './mailbox-utils';

export function OutlookMailboxProviderPanel(props: MailboxProviderPanelProps) {
  const config = mailboxProviderConfig('outlook');
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden p-3">
      <MailboxRecordList {...props} providerCapability={props.capability} showStatus={config.showStatus} emptyText={`暂无 ${config.label} 邮箱。`} />
    </div>
  );
}
