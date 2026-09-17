'use client';

import { Modal, type ModalProps } from './_modal';

const sizes = { sm: 'max-w-sm', md: 'max-w-lg', lg: 'max-w-3xl' } as const;

export type DialogProps = ModalProps & { size?: keyof typeof sizes };

/** Centred with inset + auto margins, so the scale keyframes never fight a translate (07 §8, plan I-14). */
export function Dialog({ size = 'md', ...props }: DialogProps) {
  return (
    <Modal
      {...props}
      className={`inset-0 m-auto h-fit max-h-[calc(100dvh-32px)] w-[calc(100vw-32px)] rounded-lg border ${sizes[size]} data-[state=closed]:animate-dialog-out data-[state=open]:animate-dialog-in`}
    />
  );
}
