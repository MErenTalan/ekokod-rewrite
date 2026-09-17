'use client';

import { useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useCallback } from 'react';

import { useToast } from '@/components/ui/toast';
import { api } from '@/lib/api/client';

/** Ends the session server-side, drops every cached query and returns to login. */
export function useLogout(): () => Promise<void> {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const t = useTranslations('shell.userMenu');
  return useCallback(async () => {
    try {
      const { response } = await api.POST('/api/v1/auth/logout');
      // 401: the session had already ended, which is the goal.
      if (!response.ok && response.status !== 401) throw new Error(String(response.status));
    } catch {
      toast({ tone: 'danger', title: t('logoutFailed') });
      return;
    }
    queryClient.clear();
    router.replace('/auth/login');
    router.refresh();
  }, [queryClient, router, t, toast]);
}
