'use client';

import { useTranslations } from 'next-intl';

import { Input } from '@/components/ui/input';
import { Select } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import type { Building, IntegrationCredentialCreateRequest, IntegrationDefinition, Provider } from '@/lib/api/types';

export type CredentialDraft = {
  id?: string;
  provider: Provider;
  subtype: string;
  username: string;
  secret: string;
  pm5340Url: string;
  installationNumber: string;
  isolarRegion: string;
  appKey: string;
  secretKey: string;
  appId: string;
  useBillingIndexes: boolean;
  wiringNumbers: string[];
  buildingId: string | null;
};

export const emptyDraft = (provider: Provider = 'osos', subtype = ''): CredentialDraft => ({
  provider,
  subtype,
  username: '',
  secret: '',
  pm5340Url: '',
  installationNumber: '',
  isolarRegion: '',
  appKey: '',
  secretKey: '',
  appId: '',
  useBillingIndexes: false,
  wiringNumbers: [],
  buildingId: null,
});

export const WIRING_NUMBER = /^[A-Za-z0-9._-]{1,100}$/;

/**
 * Only the fields the chosen provider actually uses reach the API (R201, R210):
 * a secret left empty on an update keeps the stored one, and wiring numbers and
 * a target building exist for gridbox (and pm5340's building) alone.
 */
export function toCredentialRequest(draft: CredentialDraft): IntegrationCredentialCreateRequest {
  const request: IntegrationCredentialCreateRequest = { provider: draft.provider, subtype: draft.subtype };
  if (draft.provider === 'isolar') {
    const extra: Record<string, string> = {};
    if (draft.appKey) extra.app_key = draft.appKey;
    if (draft.secretKey) extra.secret_key = draft.secretKey;
    if (draft.appId) extra.app_id = draft.appId;
    if (Object.keys(extra).length > 0) request.extra = extra;
    if (draft.isolarRegion) request.isolar_region = draft.isolarRegion;
    return request;
  }
  if (draft.provider === 'pm5340') {
    if (draft.pm5340Url) request.pm5340_url = draft.pm5340Url;
    if (draft.installationNumber) request.installation_number = draft.installationNumber;
    if (draft.buildingId) request.building_id = draft.buildingId;
    return request;
  }
  if (draft.username) request.username = draft.username;
  if (draft.secret) request.secret = draft.secret;
  if (draft.provider === 'gridbox') {
    request.settings = { use_billing_indexes: draft.useBillingIndexes };
    if (draft.wiringNumbers.length > 0) request.wiring_numbers = draft.wiringNumbers;
    if (draft.buildingId) request.building_id = draft.buildingId;
  }
  return request;
}

export type CredentialFormViewProps = {
  definitions: IntegrationDefinition[];
  buildings: Building[];
  value: CredentialDraft;
  onChange: (draft: CredentialDraft) => void;
  mode: 'create' | 'update';
  fieldErrors?: Record<string, string>;
};

/** The per-provider credential form of 01 §7.15. */
export function CredentialFormView({ definitions, buildings, value, onChange, mode, fieldErrors = {} }: CredentialFormViewProps) {
  const t = useTranslations('settings.credentials');
  const invalidNumber = value.wiringNumbers.find((n) => !WIRING_NUMBER.test(n));
  const duplicate = value.wiringNumbers.length !== new Set(value.wiringNumbers).size;
  const set = (patch: Partial<CredentialDraft>) => onChange({ ...value, ...patch });

  return (
    <div className="flex flex-col gap-4">
      {mode === 'create' ? (
        <Select
          label={t('definition')}
          options={definitions.map((d) => ({ value: `${d.provider}|${d.subtype}`, label: `${d.provider} · ${d.subtype}` }))}
          value={value.subtype ? `${value.provider}|${value.subtype}` : null}
          onValueChange={(choice) => {
            const [provider, subtype] = choice.split('|');
            set({ provider: provider as Provider, subtype });
          }}
          placeholder={t('choose')}
        />
      ) : null}

      {value.provider === 'isolar' ? (
        <>
          <Input label={t('appKey')} value={value.appKey} onChange={(e) => set({ appKey: e.target.value })} />
          <Input label={t('secretKey')} type="password" autoComplete="off" value={value.secretKey} onChange={(e) => set({ secretKey: e.target.value })} />
          <Input label={t('appId')} value={value.appId} onChange={(e) => set({ appId: e.target.value })} />
          <Input label={t('region')} value={value.isolarRegion} onChange={(e) => set({ isolarRegion: e.target.value })} />
        </>
      ) : value.provider === 'pm5340' ? (
        <>
          <Input label={t('pm5340Url')} value={value.pm5340Url} error={fieldErrors.pm5340_url} onChange={(e) => set({ pm5340Url: e.target.value })} />
          <Input label={t('installationNumber')} value={value.installationNumber} onChange={(e) => set({ installationNumber: e.target.value })} />
        </>
      ) : (
        <>
          <Input label={t('username')} value={value.username} error={fieldErrors.username} onChange={(e) => set({ username: e.target.value })} />
          <Input
            label={t('password')}
            type="password"
            autoComplete="off"
            value={value.secret}
            description={mode === 'update' ? t('passwordKept') : undefined}
            error={fieldErrors.secret}
            onChange={(e) => set({ secret: e.target.value })}
          />
        </>
      )}

      {value.provider === 'gridbox' ? (
        <>
          <Switch label={t('useBillingIndexes')} checked={value.useBillingIndexes} onCheckedChange={(on) => set({ useBillingIndexes: on })} />
          <Input
            label={t('wiringNumbers')}
            description={t('wiringNumbersHint')}
            value={value.wiringNumbers.join(', ')}
            error={
              fieldErrors.wiring_numbers ??
              (invalidNumber ? t('wiringNumberInvalid', { number: invalidNumber }) : duplicate ? t('wiringNumberDuplicate') : undefined)
            }
            onChange={(e) =>
              set({
                wiringNumbers: e.target.value
                  .split(',')
                  .map((n) => n.trim())
                  .filter(Boolean),
              })
            }
          />
        </>
      ) : null}

      {value.provider === 'gridbox' || value.provider === 'pm5340' ? (
        <Select
          label={t('targetBuilding')}
          options={buildings.map((b) => ({ value: b.id, label: b.name }))}
          value={value.buildingId}
          error={fieldErrors.building_id}
          onValueChange={(id) => set({ buildingId: id })}
          placeholder={t('noBuilding')}
        />
      ) : null}
    </div>
  );
}
