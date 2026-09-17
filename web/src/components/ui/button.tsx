'use client';
import { cva, type VariantProps } from 'class-variance-authority';
import { Loader2, type LucideIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { Slot } from 'radix-ui';
import type { ComponentPropsWithRef, MouseEvent, ReactNode } from 'react';
import { cn } from '@/lib/cn';

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 rounded-md font-sans font-semibold whitespace-nowrap select-none ' +
    'transition-colors duration-(--duration-hover) ease-out pointer-coarse:min-h-11 pointer-coarse:min-w-11 ' +
    'disabled:opacity-60 aria-disabled:opacity-60',
  {
    variants: {
      variant: {
        primary: 'bg-primary text-on-primary hover:bg-primary-hover',
        secondary: 'border border-border-control bg-surface text-foreground hover:bg-surface-sunken',
        ghost: 'text-foreground hover:bg-surface-sunken',
        danger: 'bg-danger text-on-danger hover:brightness-95',
      },
      size: { sm: 'h-8 px-3 type-small', md: 'h-9 px-4 type-body', lg: 'h-11 px-5 type-body-lg' },
    },
    defaultVariants: { variant: 'primary', size: 'md' },
  },
);

export type ButtonProps = ComponentPropsWithRef<'button'> & VariantProps<typeof buttonVariants> & {
  children: ReactNode;
  loading?: boolean;
  iconStart?: LucideIcon;
  iconEnd?: LucideIcon;
  /** Renders the single child (e.g. next/link) with button styling; icons and loading are ignored. */
  asChild?: boolean;
};

export function Button({ variant, size, loading = false, iconStart: IconStart, iconEnd: IconEnd, asChild = false,
  className, children, type = 'button', onClick, ...props }: ButtonProps) {
  const t = useTranslations('common');
  const classes = cn(buttonVariants({ variant, size }), className);
  if (asChild) return <Slot.Root className={classes} {...props}>{children}</Slot.Root>;
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    if (loading) { event.preventDefault(); return; }
    onClick?.(event);
  };
  return (
    <button type={type} className={classes} aria-busy={loading || undefined}
      aria-disabled={loading || undefined} onClick={handleClick} {...props}>
      {loading ? <Loader2 aria-hidden className="size-4 animate-spin" />
        : IconStart ? <IconStart aria-hidden className="size-4" /> : null}
      <span>{children}</span>
      {loading ? <span className="sr-only">{t('loading')}</span> : null}
      {IconEnd && !loading ? <IconEnd aria-hidden className="size-4" /> : null}
    </button>
  );
}
