import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';

import { renderWithProviders } from '@/test/render';

import { demoBuildingStates, demoIcmalImport } from './_fixture';
import { IcmalTabView } from './icmal-tab';

const view = (over: Partial<Parameters<typeof IcmalTabView>[0]> = {}) => {
  const onUpload = vi.fn();
  const onApply = vi.fn();
  const r = renderWithProviders(
    <IcmalTabView
      result={demoIcmalImport}
      buildings={demoBuildingStates}
      onUpload={onUpload}
      onApply={onApply}
      {...over}
    />,
  );
  return { ...r, onUpload, onApply };
};

describe('IcmalTabView', () => {
  it('writes nothing until the operator confirms a building', async () => {
    // 09 §F8's acceptance criterion, on the client (02 §8.4, R245).
    const user = userEvent.setup();
    const r = view();
    // With the date filled, the ONLY thing still holding the write back is the
    // confirmation itself.
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-10-01');
    expect(r.getByRole('button', { name: /Onayla ve uygula/ })).toBeDisabled();

    await user.click(r.getByRole('checkbox', { name: /40ZTEST000000030/ }));
    expect(r.getByRole('button', { name: /Onayla ve uygula/ })).toBeEnabled();
    expect(r.onApply).not.toHaveBeenCalled();
  });

  it('applies only the rows that were checked', async () => {
    const user = userEvent.setup();
    const r = view();
    await user.click(r.getByRole('checkbox', { name: /40ZTEST000000030/ }));
    await user.type(r.getByLabelText(/Yürürlük tarihi/), '2026-10-01');
    await user.click(r.getByRole('button', { name: /Onayla ve uygula/ }));

    expect(r.onApply).toHaveBeenCalledWith([
      { building_id: 'b-1', etso_code: '40ZTEST000000030', effective_from: '2026-10-01' },
    ]);
  });

  it('shows each coefficient with its sample count, stability and warnings', () => {
    const r = view();
    expect(r.getByText(/1\.0800/)).toBeVisible();
    expect(r.getAllByText(/6 dönem/).length).toBeGreaterThan(0);
    expect(r.getByText(/Kararsız/)).toBeVisible();
    expect(r.getByText(/güç fiyatı dönemler arasında kararsız/)).toBeVisible();
  });

  it('says when a derivation missed the 2 % target', () => {
    const r = view({
      result: {
        ...demoIcmalImport,
        analyses: [{ ...demoIcmalImport.analyses[0], within_tolerance: false }],
      },
    });
    expect(r.getByText(/%2 hedefinin dışında/)).toBeVisible();
  });

  it('lists unmatched ETSO codes instead of silently dropping them', () => {
    const r = view();
    expect(r.getByText('40Z0000000123A')).toBeVisible();
    expect(r.getByText(/hiçbir binayla eşleşmedi/)).toBeVisible();
  });

  it('cannot confirm a derivation that matched no building', () => {
    const r = view({
      result: {
        ...demoIcmalImport,
        analyses: [{ ...demoIcmalImport.analyses[0], building_id: undefined }],
      },
    });
    expect(r.getByRole('checkbox', { name: /40ZTEST000000030/ })).toBeDisabled();
  });

  it('says what went wrong when the file cannot be read', () => {
    const r = view({ result: null, failure: 'unreadable' });
    expect(r.getByRole('alert')).toHaveTextContent(/Dosya okunamadı/);
  });

  it('starts at the upload step with no result yet', () => {
    const r = view({ result: null });
    expect(r.getByLabelText(/İcmal dosyası/)).toBeVisible();
    expect(r.queryByRole('button', { name: /Onayla ve uygula/ })).toBeNull();
  });
});
