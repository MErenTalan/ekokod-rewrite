'use client';

import { useQueryClient } from '@tanstack/react-query';
import type { PathsWithMethod } from 'openapi-typescript-helpers';
import { useTranslations } from 'next-intl';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { useToast } from '@/components/ui/toast';

import { $api } from './query';
import { errorCodeOf, errorMessage, fieldErrors, requestIdOf } from './problem';
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
  // A field failure is announced after the form has rendered the marks it names.
  const pending = useRef<{ title: string; fields: Record<string, string> } | null>(null);

  const onError = useCallback(
    (error: unknown) => {
      const fields = fieldErrors(error, t);
      const title = errorMessage(error, feedback('saveFailed'));
      setErrors(fields);
      if (Object.keys(fields).length > 0) {
        pending.current = { title, fields };
        return;
      }
      toast.toast({ tone: 'danger', title, description: failureHint(error, feedback) });
    },
    [t, toast, feedback],
  );

  useEffect(() => {
    const announce = pending.current;
    if (!announce) return;
    pending.current = null;
    const marked = markedFields();
    const named = marked.length
      ? marked.map((m) => `${m.label} (${m.message})`)
      : Object.entries(announce.fields).map(([field, message]) => `${field} (${message})`);
    const shown = named.slice(0, MAX_NAMED).join(', ');
    const more = named.length > MAX_NAMED ? ` ${feedback('moreFields', { count: named.length - MAX_NAMED })}` : '';
    toast.toast({ tone: 'danger', title: announce.title, description: feedback('invalidFields', { fields: shown + more }) });
    marked[0]?.focus();
  }, [errors, toast, feedback]);

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

  // A dialog closed after a failure opens clean next time.
  const clearFieldErrors = useCallback(() => setErrors({}), []);
  return useMemo(() => ({ ...mutation, fieldErrors: errors, clearFieldErrors }), [mutation, errors, clearFieldErrors]);
}

const MAX_NAMED = 4;

/** The danger toast a hand-rolled mutation shows for a failure it does not render itself. */
export function useFailureToast(): (error: unknown) => void {
  const feedback = useTranslations('feedback');
  const toast = useToast();
  return useCallback(
    (error: unknown) => toast.toast({ tone: 'danger', title: errorMessage(error, feedback('saveFailed')), description: failureHint(error, feedback) }),
    [toast, feedback],
  );
}

type Feedback = (key: 'networkError' | 'errorReference', values?: Record<string, string>) => string;

/** What the operator can do about a failure the form cannot mark. */
function failureHint(error: unknown, feedback: Feedback): string | undefined {
  if (error instanceof Error) return feedback('networkError');
  const reference = requestIdOf(error);
  const code = errorCodeOf(error);
  // A malformed request or a server fault is not the operator's to fix: give support a handle.
  if (reference && (code === 'invalid_body' || code === 'internal')) return feedback('errorReference', { reference });
  return undefined;
}

/** The fields a form currently marks invalid, in the open dialog when there is one. */
function markedFields(): { label: string; message: string; focus: () => void }[] {
  if (typeof document === 'undefined') return [];
  const dialogs = document.querySelectorAll('[role="dialog"], [role="alertdialog"]');
  const scope = dialogs.length ? dialogs[dialogs.length - 1]! : document;
  return Array.from(scope.querySelectorAll<HTMLElement>('[data-field-error]')).map((el) => ({
    label: el.dataset.fieldLabel ?? '',
    message: el.dataset.fieldError ?? '',
    focus: () => {
      const control = el.querySelector<HTMLElement>('input, select, textarea, button, [tabindex]');
      control?.focus();
      control?.scrollIntoView?.({ block: 'center', behavior: 'smooth' });
    },
  }));
}
