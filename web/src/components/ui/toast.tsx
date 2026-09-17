'use client';

import { AlertTriangle, CheckCircle2, Info, X, XCircle } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { Toast as RadixToast } from 'radix-ui';
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';

import { cn } from '@/lib/cn';

import type { AlertTone } from './alert';
import { Button } from './button';
import { IconButton } from './icon-button';

export type ToastInput = { tone: AlertTone; title: string; description?: string; action?: { label: string; onClick: () => void } };
type ToastContextValue = { toast: (input: ToastInput) => void };

const ToastContext = createContext<ToastContextValue | null>(null);
const tones = {
  info: { edge: 'border-s-info', icon: 'text-info', Icon: Info },
  success: { edge: 'border-s-success', icon: 'text-success', Icon: CheckCircle2 },
  warning: { edge: 'border-s-warning', icon: 'text-warning', Icon: AlertTriangle },
  danger: { edge: 'border-s-danger', icon: 'text-danger', Icon: XCircle },
} as const;

/** Bottom-end toasts, 6 s, paused while hovered or focused (Radix). Mounted once in AppProviders. */
export function Toaster({ children }: { children: ReactNode }) {
  const t = useTranslations('feedback');
  const [toasts, setToasts] = useState<(ToastInput & { id: number })[]>([]);
  const toast = useCallback((input: ToastInput) => setToasts((list) => [...list, { ...input, id: Date.now() + Math.random() }]), []);
  const value = useMemo(() => ({ toast }), [toast]);
  return (
    <RadixToast.Provider duration={6000} swipeDirection="right" label={t('notification')}>
      <ToastContext.Provider value={value}>{children}</ToastContext.Provider>
      {toasts.map((item) => {
        const { edge, icon, Icon } = tones[item.tone];
        return (
        <RadixToast.Root
          key={item.id}
          type={item.tone === 'danger' || item.tone === 'warning' ? 'foreground' : 'background'}
          onOpenChange={(open) => {
            if (!open) setToasts((list) => list.filter((x) => x.id !== item.id));
          }}
          className={cn(
            'flex items-start gap-3 rounded-md border border-s-4 border-border bg-surface-raised p-3 text-foreground shadow-lg data-[state=closed]:animate-fade-out data-[state=open]:animate-popover-in',
            edge,
          )}
        >
          <Icon aria-hidden className={cn('mt-0.5 size-5 shrink-0', icon)} />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <RadixToast.Title className="flex items-center gap-2 font-semibold type-body">
              {item.title}
            </RadixToast.Title>
            {item.description ? <RadixToast.Description className="text-foreground-muted type-small">{item.description}</RadixToast.Description> : null}
            {item.action ? (
              <RadixToast.Action altText={item.action.label} asChild>
                <Button size="sm" variant="secondary" className="self-start" onClick={item.action.onClick}>
                  {item.action.label}
                </Button>
              </RadixToast.Action>
            ) : null}
          </div>
          <RadixToast.Close asChild>
            <IconButton label={t('dismiss')} icon={X} size="sm" tooltip={false} />
          </RadixToast.Close>
        </RadixToast.Root>
        );
      })}
      <RadixToast.Viewport
        label={t('notifications', { hotkey: '{hotkey}' })}
        className="fixed end-0 bottom-0 z-50 flex max-h-dvh w-full max-w-sm flex-col gap-2 p-4"
      />
    </RadixToast.Provider>
  );
}

export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) throw new Error('useToast must be used inside <Toaster> (AppProviders)');
  return context;
}
