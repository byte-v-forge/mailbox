import { Inbox } from 'lucide-react';
import {
  Alert,
  AlertDescription,
  Badge,
  Button,
  EmptyBlock,
  Item,
  ItemContent,
  ItemDescription,
  ItemTitle,
  compactToast,
  formatUnix,
  mask,
  maskPreview
} from '@byte-v-forge/common-ui';
import { formatEmailList, maskEmail } from './email-utils';
import { messageSignals, signalKindName, signalLabel, verificationCodeForMessage } from './mailbox-signal-utils';
import type { InboxMessage, InboxResult, Mailbox } from './types';

export function MailboxInboxSection({ mailbox, result, showSecrets, loading, canFetch, onFetch }: {
  mailbox: Mailbox;
  result?: InboxResult | null;
  showSecrets: boolean;
  loading: boolean;
  canFetch: boolean;
  onFetch: (emailAddress?: string) => Promise<void>;
}) {
  const messages = result?.messages || [];
  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">收件箱</h3>
        {canFetch && (
          <Button variant="outline" size="sm" disabled={loading} onClick={() => onFetch(mailbox.email_address)}>
            <Inbox />{loading ? '刷新中' : '刷新'}
          </Button>
        )}
      </div>
      {result?.error_message && (
        <Alert variant="destructive">
          <AlertDescription>{compactToast(result.error_message)}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-2">
        {messages.map((message, index) => (
          <InboxMessageRow message={message} showSecrets={showSecrets} key={`${message.mailbox_email}-${message.id || index}`} />
        ))}
        {!result && <EmptyBlock text={loading ? '正在读取收件箱。' : '暂无邮件。'} />}
        {result && !result.error_message && messages.length === 0 && <EmptyBlock text="当前邮箱没有新邮件。" />}
      </div>
    </section>
  );
}

function InboxMessageRow({ message, showSecrets }: {
  message: InboxMessage;
  showSecrets: boolean;
}) {
  return (
    <Item variant="outline" className="items-start">
      <ItemContent className="min-w-0">
        <ItemTitle className="w-full justify-between gap-2">
          <span className="truncate" title={message.subject}>{message.subject || '-'}</span>
          <span className="shrink-0 text-xs font-normal text-muted-foreground">{formatUnix(message.received_at_unix)}</span>
        </ItemTitle>
        <ItemDescription className="flex items-center justify-between gap-2">
          <span className="truncate">发件人 {showSecrets ? (message.from_address || '-') : maskEmail(message.from_address)}</span>
          <MessageSignalStrip message={message} showSecrets={showSecrets} />
        </ItemDescription>
        <ItemDescription className="line-clamp-1" title={formatEmailList(message.recipients, true)}>
          收件人 {formatEmailList(message.recipients, showSecrets)}
        </ItemDescription>
        <ItemDescription className="line-clamp-3">{showSecrets ? (message.body_preview || '-') : maskPreview(message.body_preview || '-')}</ItemDescription>
      </ItemContent>
    </Item>
  );
}

function MessageSignalStrip({ message, showSecrets }: {
  message: InboxMessage;
  showSecrets: boolean;
}) {
  const signals = messageSignals(message);
  const fallbackCode = verificationCodeForMessage(message);
  if (signals.length === 0 && fallbackCode) {
    return <Badge variant="outline" className="border-emerald-200 bg-emerald-50 font-mono text-emerald-700">验证码 {showSecrets ? fallbackCode : mask(fallbackCode)}</Badge>;
  }
  if (signals.length === 0) return null;
  return (
    <span className="flex shrink-0 items-center gap-1">
      {signals.map((signal, index) => {
        const kind = signalKindName(signal.kind);
        const code = kind === 'otp' && signal.code ? ` ${showSecrets ? signal.code : mask(signal.code)}` : '';
        return (
          <Badge variant="secondary" key={`${kind}-${signal.code || signal.label || index}`}>
            {signalLabel(signal)}{code}
          </Badge>
        );
      })}
    </span>
  );
}
