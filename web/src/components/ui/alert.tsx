import { AlertTriangle, CheckCircle2, Info, XCircle } from 'lucide-react';
import type { ReactNode } from 'react';

import { cn } from '@/lib/cn';

export type AlertTone = 'info' | 'success' | 'warning' | 'danger';

const styles = {
  info: { box: 'border-info bg-info-subtle', icon: 'text-info', Icon: Info },
  success: { box: 'border-success bg-success-subtle', icon: 'text-success', Icon: CheckCircle2 },
  warning: { box: 'border-warning bg-warning-subtle', icon: 'text-warning', Icon: AlertTriangle },
  danger: { box: 'border-danger bg-danger-subtle', icon: 'text-danger', Icon: XCircle },
} as const;

export type AlertProps = { tone: AlertTone; title: string; children?: ReactNode; action?: ReactNode; className?: string };

/** Warning and danger interrupt (role=alert); info and success wait their turn (role=status). */
export function Alert({ tone, title, children, action, className }: AlertProps) {
  const { box, icon, Icon } = styles[tone];
  return (
    <div
      role={tone === 'warning' || tone === 'danger' ? 'alert' : 'status'}
      className={cn('flex flex-wrap items-start gap-3 rounded-md border p-3 text-foreground', box, className)}
    >
      <Icon aria-hidden className={cn('mt-0.5 size-5 shrink-0', icon)} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <p className="font-semibold type-body">{title}</p>
        {children ? <div className="type-small">{children}</div> : null}
      </div>
      {action ? <div className="flex shrink-0 items-center gap-2">{action}</div> : null}
    </div>
  );
}
