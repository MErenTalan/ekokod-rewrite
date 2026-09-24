'use client';

import { X } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { Dialog as RadixDialog } from 'radix-ui';
import { useRef, type ReactElement, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

import { IconButton } from './icon-button';

export type ModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  trigger?: ReactElement;
};

/** Radix Dialog shell shared by Dialog and Drawer: overlay, title, close button, scrollable body, footer. */
export function Modal({ open, onOpenChange, title, description, children, footer, trigger, className, side }: ModalProps & { className: string; side?: string }) {
  const t = useTranslations('feedback');
  const opener = useRef<HTMLElement | null>(null);
  return (
    <RadixDialog.Root open={open} onOpenChange={onOpenChange}>
      {trigger ? <RadixDialog.Trigger asChild>{trigger}</RadixDialog.Trigger> : null}
      <RadixDialog.Portal>
        <RadixDialog.Overlay className="fixed inset-0 z-50 bg-overlay data-[state=closed]:animate-fade-out data-[state=open]:animate-fade-in" />
        <RadixDialog.Content
          data-side={side}
          {...(description ? {} : { 'aria-describedby': undefined })}
          // Without a Trigger, Radix cannot restore focus on close; remember the opener ourselves (plan I-10).
          onOpenAutoFocus={() => {
            opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          }}
          onCloseAutoFocus={(event) => {
            if (trigger) return;
            event.preventDefault();
            if (opener.current?.isConnected) opener.current.focus();
          }}
          className={cn('fixed z-50 flex flex-col border-border bg-surface-raised text-foreground shadow-lg', className)}
        >
          <div className="flex items-start gap-3 px-4 pt-4 pb-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <RadixDialog.Title className="type-h2">{title}</RadixDialog.Title>
              {description ? <RadixDialog.Description className="text-foreground-muted type-body">{description}</RadixDialog.Description> : null}
            </div>
            <RadixDialog.Close asChild>
              <IconButton label={t('close')} icon={X} size="sm" tooltip={false} className="-me-1" />
            </RadixDialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">{children}</div>
          {footer ? <div className="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">{footer}</div> : null}
        </RadixDialog.Content>
      </RadixDialog.Portal>
    </RadixDialog.Root>
  );
}
