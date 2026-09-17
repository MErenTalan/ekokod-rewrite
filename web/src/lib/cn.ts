import { clsx, type ClassValue } from 'clsx';
import { extendTailwindMerge } from 'tailwind-merge';

// Teach tailwind-merge the custom type-* utilities so they conflict with each other, not with text colours.
const twMerge = extendTailwindMerge({
  extend: {
    classGroups: {
      'font-size': [{ type: ['display', 'h1', 'h2', 'h3', 'body', 'body-lg', 'small', 'caption', 'metric', 'data'] }],
    },
  },
});

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
