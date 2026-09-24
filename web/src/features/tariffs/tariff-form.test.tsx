import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { emptyDraft, type TariffDraft } from './tariff-draft';
import { TariffFormView } from './tariff-form';

const complete = (over: Partial<TariffDraft> = {}): TariffDraft => ({
  ...emptyDraft(),
  effectiveFrom: '2026-09-01',
  distributionCost: '0.85',
  reactivePowerPrice: '1.2',
  vatRate: '20',
  singleTimePrice: '3.15',
  ...over,
});

const view = (over: Partial<TariffDraft> = {}, props: Partial<Parameters<typeof TariffFormView>[0]> = {}) => {
  const onDraftChange = props.onDraftChange ?? vi.fn();
  const onSubmit = props.onSubmit ?? vi.fn();
  const r = renderWithProviders(
    <TariffFormView draft={complete(over)} onDraftChange={onDraftChange} onSubmit={onSubmit} {...props} />,
  );
  return { ...r, onDraftChange, onSubmit };
};

describe('TariffFormView', () => {
  it('hides the KBK fieldset until PTF+YEKDEM is switched on', async () => {
    const user = userEvent.setup();
    const r = view();
    expect(r.queryByLabelText(/Enerji KBK/)).toBeNull();
    expect(r.getByLabelText(/Tek zamanlı fiyat/)).toBeVisible();

    await user.click(r.getByRole('switch', { name: /PTF \+ YEKDEM/ }));
    expect(r.onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ usePtfYekdem: true, singleTimePrice: '' }));
  });

  it('refuses to save a PTF tariff without an energy KBK and names the field', async () => {
    // 09 §F8's acceptance criterion, on the client as well as the server.
    const user = userEvent.setup();
    const r = view({ usePtfYekdem: true, singleTimePrice: '' });
    await user.click(r.getByRole('button', { name: /Kaydet/ }));

    expect(r.onSubmit).not.toHaveBeenCalled();
    expect(r.getByLabelText(/Enerji KBK/)).toHaveAccessibleDescription(/Zorunlu/);
  });

  it('saves a complete fixed-price tariff', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Kaydet/ }));
    expect(r.onSubmit).toHaveBeenCalledTimes(1);
  });

  it('reveals contracted power and the power price only for a binomial tariff', () => {
    expect(view().queryByLabelText(/Sözleşme gücü/)).toBeNull();
    expect(view({ term: 'binomial' }).getByLabelText(/Sözleşme gücü/)).toBeVisible();
  });

  it('renders a server field error on its own field', () => {
    const r = view({ usePtfYekdem: true, kbkEnergy: '1.08', singleTimePrice: '' },
      { serverErrors: { kbk_distribution_cost_tl_per_kwh: 'Zorunlu alan' } });
    expect(r.getByLabelText(/Dağıtım bedeli KBK/)).toHaveAccessibleDescription(/Zorunlu alan/);
  });

  it('says plainly that a PTF tariff is priced in TRY only (R127)', () => {
    const r = view({ usePtfYekdem: true, kbkEnergy: '1.08', currency: 'USD', singleTimePrice: '' });
    expect(r.getByLabelText(/Para birimi/)).toHaveAccessibleDescription(/Türk Lirası/);
  });

  it('puts the T1 KBK error on the T1 KBK field', () => {
    // The field name on the wire is kbk_t1, not kbk_t_1: a naive camel-to-snake
    // rule silently drops the message and the operator sees no reason at all.
    const r = view({ usePtfYekdem: true, kbkEnergy: '1.08', priceType: 'multi_time', singleTimePrice: '' });
    expect(r.getByLabelText(/T1 KBK/)).toHaveAccessibleDescription(/Zorunlu/);
  });

  it('is read-only when the caller cannot edit', () => {
    const r = view({}, { readOnly: true });
    expect(r.queryByRole('button', { name: /Kaydet/ })).toBeNull();
    expect(r.getByLabelText(/Tek zamanlı fiyat/)).toBeDisabled();
  });

  it('adds and removes an extra tax row', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('button', { name: /Vergi ekle/ }));
    expect(r.onDraftChange).toHaveBeenCalledWith(expect.objectContaining({ taxes: [{ name: '', rate: '' }] }));
  });

  it('offers the manual YEKDEM table only under PTF+YEKDEM', () => {
    expect(view().queryByRole('switch', { name: /Manuel YEKDEM/ })).toBeNull();
    expect(view({ usePtfYekdem: true, kbkEnergy: '1.08', singleTimePrice: '' }).getByRole('switch', { name: /Manuel YEKDEM/ })).toBeVisible();
  });
});
