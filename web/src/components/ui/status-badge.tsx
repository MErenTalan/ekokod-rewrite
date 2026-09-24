import { AlertTriangle, CheckCircle2, CircleDashed, Info, XCircle, type LucideIcon } from 'lucide-react';

import { Badge } from './badge';

export type Status = 'success' | 'warning' | 'danger' | 'info' | 'neutral';

const icons: Record<Status, LucideIcon> = {
  success: CheckCircle2,
  warning: AlertTriangle,
  danger: XCircle,
  info: Info,
  neutral: CircleDashed,
};

/** Colour + icon + text, never colour alone (07 §2.4). */
export function StatusBadge({ status, label }: { status: Status; label: string }) {
  const Icon = icons[status];
  return (
    <Badge tone={status}>
      <Icon aria-hidden className="size-3.5 shrink-0" />
      <span>{label}</span>
    </Badge>
  );
}
