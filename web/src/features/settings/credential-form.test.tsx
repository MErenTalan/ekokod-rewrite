import { describe, expect, it } from 'vitest';

import type { Building, IntegrationDefinition } from '@/lib/api/types';
import { renderWithProviders } from '@/test/render';

import { CredentialFormView, emptyDraft, toCredentialRequest, type CredentialDraft } from './credential-form';

const definitions = [
  { id: 'd-1', provider: 'osos', subtype: 'Baskent', endpoints: {}, updated_at: '' },
  { id: 'd-2', provider: 'gridbox', subtype: 'default', endpoints: {}, updated_at: '' },
  { id: 'd-3', provider: 'pm5340', subtype: 'default', endpoints: {}, updated_at: '' },
  { id: 'd-4', provider: 'isolar', subtype: 'eu', endpoints: {}, updated_at: '' },
] as IntegrationDefinition[];
const buildings = [{ id: 'b-1', name: 'A1 Fabrika' }] as Building[];

const draft = (overrides: Partial<CredentialDraft>): CredentialDraft => ({ ...emptyDraft(), ...overrides });

describe('toCredentialRequest', () => {
  it('sends a gridbox credential with its wiring numbers and target building (R210)', () => {
    expect(
      toCredentialRequest(
        draft({
          provider: 'gridbox',
          subtype: 'default',
          username: 'gb',
          secret: 'pw',
          useBillingIndexes: true,
          wiringNumbers: ['1001', '1002'],
          buildingId: 'b-1',
        }),
      ),
    ).toEqual({
      provider: 'gridbox',
      subtype: 'default',
      username: 'gb',
      secret: 'pw',
      settings: { use_billing_indexes: true },
      wiring_numbers: ['1001', '1002'],
      building_id: 'b-1',
    });
  });

  it('never sends wiring numbers or a building for a discovering provider', () => {
    const request = toCredentialRequest(
      draft({ provider: 'osos', subtype: 'Baskent', username: 'o', secret: 'p', wiringNumbers: ['1001'], buildingId: 'b-1' }),
    );
    expect(request).toEqual({ provider: 'osos', subtype: 'Baskent', username: 'o', secret: 'p' });
  });

  it('sends pm5340 by address and installation number, isolar by its extra keys', () => {
    expect(
      toCredentialRequest(draft({ provider: 'pm5340', subtype: 'default', pm5340Url: 'http://10.0.0.5:502', installationNumber: 'PM-1', buildingId: 'b-1' })),
    ).toEqual({ provider: 'pm5340', subtype: 'default', pm5340_url: 'http://10.0.0.5:502', installation_number: 'PM-1', building_id: 'b-1' });

    expect(
      toCredentialRequest(draft({ provider: 'isolar', subtype: 'eu', appKey: 'ak', secretKey: 'sk', appId: 'ai', isolarRegion: 'eu' })),
    ).toEqual({ provider: 'isolar', subtype: 'eu', extra: { app_key: 'ak', secret_key: 'sk', app_id: 'ai' }, isolar_region: 'eu' });
  });

  it('leaves an empty secret out, so the stored one is kept', () => {
    const request = toCredentialRequest(draft({ provider: 'osos', subtype: 'Baskent', username: 'o', secret: '' }));
    expect(request).not.toHaveProperty('secret');
  });
});

describe('CredentialFormView', () => {
  it('shows only the fields of the chosen provider', () => {
    const gridbox = renderWithProviders(
      <CredentialFormView
        definitions={definitions}
        buildings={buildings}
        value={draft({ provider: 'gridbox', subtype: 'default' })}
        onChange={() => {}}
        mode="create"
      />,
    );
    expect(gridbox.getByLabelText(/Kablo \(wiring\) numaraları/)).toBeInTheDocument();
    expect(gridbox.getByLabelText(/Hedef bina/)).toBeInTheDocument();
    expect(gridbox.queryByLabelText(/App key/)).toBeNull();
    gridbox.unmount();

    const isolar = renderWithProviders(
      <CredentialFormView
        definitions={definitions}
        buildings={buildings}
        value={draft({ provider: 'isolar', subtype: 'eu' })}
        onChange={() => {}}
        mode="create"
      />,
    );
    expect(isolar.getByLabelText(/App key/)).toBeInTheDocument();
    expect(isolar.queryByLabelText(/Kablo/)).toBeNull();
    expect(isolar.queryByLabelText(/Hedef bina/)).toBeNull();
  });

  it('refuses a malformed or duplicated wiring number before submitting', () => {
    const bad = renderWithProviders(
      <CredentialFormView
        definitions={definitions}
        buildings={buildings}
        value={draft({ provider: 'gridbox', subtype: 'default', wiringNumbers: ['1001; drop'] })}
        onChange={() => {}}
        mode="create"
      />,
    );
    expect(bad.getByText('Geçersiz numara: 1001; drop')).toBeInTheDocument();
    bad.unmount();

    const duplicate = renderWithProviders(
      <CredentialFormView
        definitions={definitions}
        buildings={buildings}
        value={draft({ provider: 'gridbox', subtype: 'default', wiringNumbers: ['1001', '1001'] })}
        onChange={() => {}}
        mode="create"
      />,
    );
    expect(duplicate.getByText('Aynı numara iki kez girilmiş')).toBeInTheDocument();
  });
});
