'use client';

import { useQueryClient } from '@tanstack/react-query';
import { useTranslations } from 'next-intl';
import { useEffect } from 'react';

import { useToast } from '@/components/ui/toast';

import { errorCodeOf, errorMessage } from './problem';

/**
 * A failed read is said once, everywhere (07 §11): without this a screen whose
 * request failed looks exactly like a screen with no data. Mutations carry
 * their own feedback (R199), so only queries are watched here, and each failed
 * query speaks once — a retry that fails again does not repeat itself.
 *
 * A refused request says nothing: what a role may not read is already hidden,
 * and the session layer owns 401.
 */
export function QueryErrorToaster() {
  const queryClient = useQueryClient();
  const feedback = useTranslations('feedback');
  const toast = useToast();

  useEffect(() => {
    const spoken = new Set<string>();
    return queryClient.getQueryCache().subscribe((event) => {
      const query = event.query;
      const { status, error, fetchStatus } = query.state;
      if (status === 'success' || (status === 'pending' && fetchStatus === 'fetching')) {
        spoken.delete(query.queryHash);
        return;
      }
      if (status !== 'error' || !error || spoken.has(query.queryHash)) return;
      // The thrown value is the API's error envelope, not an HTTP-aware error,
      // so the code is what tells a refusal from a failure.
      const code = errorCodeOf(error);
      if (code === 'unauthorized' || code === 'forbidden') return;
      // A screen that shows "nothing yet" for these codes lists them in the query's meta.
      const quiet = (query.meta as { quietErrors?: string[] } | undefined)?.quietErrors;
      if (code && quiet?.includes(code)) return;
      spoken.add(query.queryHash);
      toast.toast({ tone: 'danger', title: errorMessage(error, feedback('loadFailed')) });
    });
  }, [queryClient, toast, feedback]);

  return null;
}
