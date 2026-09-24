import { cva } from 'class-variance-authority';

// Server-safe: a server component may style a Link as a button (AuthMessage).
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
