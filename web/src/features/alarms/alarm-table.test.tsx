import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoAlarms } from './_fixture';
import { AlarmTableView, type AlarmTableViewProps } from './alarm-table';

const view = (props: Partial<AlarmTableViewProps> = {}) => (
  <AlarmTableView
    alarms={demoAlarms}
    state="all"
    onStateChange={() => {}}
    canEdit
    canEvaluate
    onCreate={() => {}}
    onEdit={() => {}}
    onDelete={() => {}}
    onToggle={() => {}}
    onDetails={() => {}}
    onEvaluate={() => {}}
    {...props}
  />
);

describe('AlarmTableView', () => {
  it('shows each rule with its type and analyzers (§7.12)', () => {
    const r = renderWithProviders(view());
    expect(r.getByText('Endüktif izleme')).toBeVisible();
    expect(r.getByText('Reaktif Limit Algılama Alarmı')).toBeVisible();
    expect(r.getByText('A-1')).toBeVisible();
  });

  it('labels the power type without promising voltage or current (R212)', () => {
    const power = [{ ...demoAlarms[0], id: 'al-3', type: 'current_voltage_power' }] as typeof demoAlarms;
    const r = renderWithProviders(view({ alarms: power }));
    expect(r.getByText('Güç Alarmı')).toBeVisible();
    expect(r.queryByText(/Voltaj/)).toBeNull();
  });

  it('reports the state filter', async () => {
    const onStateChange = vi.fn();
    const r = renderWithProviders(view({ onStateChange }));
    await r.user.click(r.getByRole('radio', { name: 'Pasif' }));
    expect(onStateChange).toHaveBeenCalledWith('passive');
  });

  it('hides create, edit, delete and evaluate from a read-only role', () => {
    const r = renderWithProviders(view({ canEdit: false, canEvaluate: false }));
    expect(r.queryByRole('button', { name: 'Yeni Alarm Ekle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Düzenle' })).toBeNull();
    expect(r.queryByRole('button', { name: 'Şimdi değerlendir' })).toBeNull();
    // Details is a read, so it stays for everyone.
    expect(r.getAllByRole('button', { name: 'Detaylar' })).toHaveLength(demoAlarms.length);
  });

  it('disables the toggle without alarms.edit', () => {
    const r = renderWithProviders(view({ canEdit: false }));
    expect(r.getAllByRole('switch')[0]).toBeDisabled();
  });

  it('says what is missing when there is nothing to show', () => {
    const r = renderWithProviders(view({ alarms: [] }));
    expect(r.getByText('Henüz alarm kuralı yok.')).toBeVisible();
    const filtered = renderWithProviders(view({ alarms: [], state: 'active' }));
    expect(filtered.getByText('Filtre kriterlerine uygun alarm bulunamadı.')).toBeVisible();
  });
});
