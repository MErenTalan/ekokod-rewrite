import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoSolarTariffs } from './_fixture';
import { SolarTabView } from './solar-tab';

const plants = [{ id: 'p-1', name: 'Çatı GES' }];

const view = (over: Partial<Parameters<typeof SolarTabView>[0]> = {}) => {
  const onCreate = vi.fn();
  const r = renderWithProviders(
    <SolarTabView
      plants={plants}
      plantID="p-1"
      onPlantChange={() => {}}
      tariffs={demoSolarTariffs}
      canEdit
      onCreate={onCreate}
      onDelete={() => {}}
      {...over}
    />,
  );
  return { ...r, onCreate };
};

describe('SolarTabView', () => {
  it('asks for a plant before it shows any tariff history', () => {
    const r = view({ plantID: null });
    expect(r.getByText(/bir santral seçin/i)).toBeVisible();
    expect(r.queryByRole('table')).toBeNull();
  });

  it('renders effective dates in the house format and sends ISO (05 §1)', async () => {
    const user = userEvent.setup();
    const r = view();
    expect(r.getByRole('cell', { name: /Oca 2026/ })).toBeVisible();

    await user.click(r.getByRole('button', { name: /Yeni tarife/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-09-01');
    await user.type(r.getByLabelText(/Feed-in tarifesi/), '2.75');
    await user.click(r.getByRole('button', { name: /Kaydet/ }));

    expect(r.onCreate).toHaveBeenCalledWith(expect.objectContaining({ effective_from: '2026-09-01', feed_in_tariff: '2.75' }));
  });

  it('offers an add-first-tariff action when the plant has none', () => {
    const r = view({ tariffs: [] });
    expect(r.getByRole('button', { name: /İlk tarifeyi ekle/ })).toBeVisible();
  });

  it('offers no write action to a read-only principal', () => {
    const r = view({ canEdit: false });
    expect(r.queryByRole('button', { name: /Yeni tarife/ })).toBeNull();
    expect(r.queryByRole('button', { name: /Sil/ })).toBeNull();
  });
});
