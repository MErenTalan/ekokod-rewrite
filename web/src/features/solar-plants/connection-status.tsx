'use client';

import { useTranslations } from 'next-intl';

import { StatusBadge } from '@/components/ui/status-badge';
import type { PlantRealtime } from '@/lib/api/types';

import { shortDateTime } from './format';

const TONE = { connected: 'success', error: 'danger', never_synced: 'neutral' } as const;
const LABEL = { connected: 'connected', error: 'error', never_synced: 'neverSynced' } as const;
export const SYNC_ERRORS = {
  isolar_auth: 'isolarAuth', isolar_unavailable: 'isolarUnavailable', isolar_not_linked: 'isolarNotLinked', credential_missing: 'credentialMissing',
} as const;

/** §7.7's connection indicator (R282): the chip plus the closed error code's sentence. */
export function ConnectionStatus({ realtime }: { realtime?: PlantRealtime }) {
  const t = useTranslations('solarPlants');
  if (!realtime) return null;
  const code = realtime.connection_error;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <StatusBadge status={TONE[realtime.connection]} label={t(`connection.${LABEL[realtime.connection]}`)} />
      {realtime.last_sync_at ? (
        <span className="text-foreground-muted type-caption">{t('lastSync', { time: shortDateTime(realtime.last_sync_at) })}</span>
      ) : null}
      {code && code in SYNC_ERRORS ? (
        <span className="text-foreground type-caption">{t(`syncErrors.${SYNC_ERRORS[code as keyof typeof SYNC_ERRORS]}`)}</span>
      ) : null}
    </div>
  );
}
