import { describe, expect, it, vi } from 'vitest';

import { renderWithProviders } from '@/test/render';

import { demoRuns } from './_fixture';
import { JobRunsTableView, type JobRunsTableViewProps } from './job-runs-table';

const TRIGGERABLE = ['alarm.evaluate', 'billing.dispatch'] as const;

const view = (props: Partial<JobRunsTableViewProps> = {}) => (
  <JobRunsTableView
    runs={demoRuns}
    triggerable={TRIGGERABLE}
    canTrigger
    onTrigger={() => {}}
    {...props}
  />
);

describe('JobRunsTableView', () => {
  it('shows each run with its result and the three counts', () => {
    const r = renderWithProviders(view());
    expect(r.getByText('alarm.evaluate')).toBeVisible();
    expect(r.getByText('Kısmi')).toBeVisible();
    expect(r.getByText('12')).toBeVisible();
    expect(r.getByText('3')).toBeVisible();
    expect(r.getByText('1')).toBeVisible();
  });

  it('shows a still-running job without a finish time', () => {
    const r = renderWithProviders(view());
    expect(r.getByText('Çalışıyor')).toBeVisible();
    expect(r.getByText('—')).toBeVisible();
  });

  it('offers one button per allow-listed job to an admin (R220)', async () => {
    const onTrigger = vi.fn();
    const r = renderWithProviders(view({ onTrigger }));
    await r.user.click(r.getByRole('button', { name: /alarm\.evaluate/ }));
    expect(onTrigger).toHaveBeenCalledWith('alarm.evaluate');
    expect(r.getAllByRole('button', { name: /Şimdi çalıştır/ })).toHaveLength(TRIGGERABLE.length);
  });

  it('hides every trigger from a company admin', () => {
    const r = renderWithProviders(view({ canTrigger: false }));
    expect(r.queryByRole('button', { name: /Şimdi çalıştır/ })).toBeNull();
    // The history itself stays readable.
    expect(r.getByText('alarm.evaluate')).toBeVisible();
  });

  it('says nothing has run when the history is empty', () => {
    const r = renderWithProviders(view({ runs: [] }));
    expect(r.getByText('Henüz iş kaydı yok.')).toBeVisible();
  });
});
