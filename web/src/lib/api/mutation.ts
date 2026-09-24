'use client';

import { useQueryClient } from '@tanstack/react-query';
import type { PathsWithMethod } from 'openapi-typescript-helpers';
import { useTranslations } from 'next-intl';
import { useCallback, useMemo, useState } from 'react';

import { useToast } from '@/components/ui/toast';

import { $api } from './query';
import { errorMessage, fieldErrors } from './problem';
import type { paths } from './schema';
import type { Translator } from './types';

type Method = 'post' | 'put' | 'patch' | 'delete';

export type ApiMutationOptions = {
  /** Toast title on success; omitted means no toast (the caller shows its own). */
  success?: string;
  /** Query key path prefixes to invalidate, e.g. ['/api/v1/buildings']. */
  invalidate?: string[];
  onSuccess?: () => void;
};

/**
 * One mutation convention for every screen (R199): a success toast, a danger
 * toast carrying the API's own localised message, 422 `details` mapped to field
 * errors, and invalidation by path prefix.
 */
export function useApiMutation<M extends Method, P extends PathsWithMethod<paths, M>>(
  method: M,
  path: P,
  options: ApiMutationOptions = {},
) {
  const t = useTranslations('forms') as unknown as Translator;
  const feedback = useTranslations('feedback');
  const toast = useToast();
  const queryClient = useQueryClient();
  const [errors, setErrors] = useState<Record<string, string>>({});

  const onError = useCallback(
    (error: unknown) => {
      setErrors(fieldErrors(error, t));
      toast.toast({ tone: 'danger', title: errorMessage(error, feedback('saveFailed')) });
    },
    [t, toast, feedback],
  );

  const mutation = $api.useMutation(method, path, {
    onSuccess: () => {
      setErrors({});
      if (options.success) toast.toast({ tone: 'success', title: options.success });
      for (const prefix of options.invalidate ?? []) {
        void queryClient.invalidateQueries({
          predicate: (query) => JSON.stringify(query.queryKey).includes(prefix),
        });
      }
      options.onSuccess?.();
    },
    onError,
  });

  return useMemo(() => ({ ...mutation, fieldErrors: errors }), [mutation, errors]);
}
