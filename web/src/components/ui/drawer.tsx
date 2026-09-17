'use client';

import { Modal, type ModalProps } from './_modal';

const sides = {
  start: 'inset-y-0 start-0 h-full w-[min(360px,calc(100vw-48px))] border-e data-[state=closed]:animate-drawer-out-start data-[state=open]:animate-drawer-in-start',
  end: 'inset-y-0 end-0 h-full w-[min(400px,calc(100vw-48px))] border-s data-[state=closed]:animate-drawer-out-end data-[state=open]:animate-drawer-in-end',
  bottom: 'inset-x-0 bottom-0 max-h-[85dvh] rounded-t-lg border-t data-[state=closed]:animate-drawer-out-bottom data-[state=open]:animate-drawer-in-bottom',
} as const;

export type DrawerProps = ModalProps & { side: keyof typeof sides };

export function Drawer({ side, ...props }: DrawerProps) {
  return <Modal {...props} side={side} className={sides[side]} />;
}
