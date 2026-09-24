'use client';

import { useEffect, useRef } from 'react';

import { Alert, type AlertTone } from '@/components/ui/alert';

/** A form-level message that takes focus when it appears, so screen-reader and keyboard users land on it. */
export function AuthFeedback({ tone, message }: { tone: AlertTone; message: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [message]);
  return (
    <div ref={ref} tabIndex={-1} className="rounded-md">
      <Alert tone={tone} title={message} />
    </div>
  );
}
