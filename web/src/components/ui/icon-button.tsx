'use client';

import type { VariantProps } from 'class-variance-authority';
import type { LucideIcon } from 'lucide-react';
import type { ComponentPropsWithRef } from 'react';

import { cn } from '@/lib/cn';

import { buttonVariants } from './button';
import { Tooltip } from './tooltip';

const square = { sm: 'size-8 px-0', md: 'size-9 px-0', lg: 'size-11 px-0' } as const;

export type IconButtonProps = Omit<ComponentPropsWithRef<'button'>, 'children'> &
  VariantProps<typeof buttonVariants> & {
    /** Accessible name and tooltip text (07 §9: icon-only buttons are always labelled). */
    label: string;
    icon: LucideIcon;
  };

export function IconButton({ label, icon: Icon, variant = 'ghost', size = 'md', className, type = 'button', ...props }: IconButtonProps) {
  return (
    <Tooltip content={label}>
      <button
        type={type}
        aria-label={label}
        className={cn(buttonVariants({ variant, size }), square[size ?? 'md'], className)}
        {...props}
      >
        <Icon aria-hidden className={size === 'lg' ? 'size-5' : 'size-4'} />
      </button>
    </Tooltip>
  );
}
