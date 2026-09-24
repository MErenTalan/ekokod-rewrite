import { Inbox, type LucideIcon } from 'lucide-react';
import type { ReactNode } from 'react';

export type EmptyStateProps = { icon?: LucideIcon; title: string; description: string; action?: ReactNode };

/** Says what is missing and what to do next (07 §5, §11). */
export function EmptyState({ icon: Icon = Inbox, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-8 text-center">
      <Icon aria-hidden className="size-8 text-foreground-muted" />
      <p className="text-foreground type-h3">{title}</p>
      <p className="max-w-md text-foreground-muted type-body">{description}</p>
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}
