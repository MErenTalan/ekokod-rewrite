import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { JobStatusBanner, type JobView } from './job-status-banner';

const job = (status: JobView['status'], extra: Partial<JobView> = {}): JobView => ({ id: 'j1', label: 'Ağustos faturaları', status, ...extra });

describe('JobStatusBanner', () => {
  it('failure is announced assertively', async () => {
    const { rerender, baseElement, getByRole } = renderWithProviders(<JobStatusBanner job={job('running', { progress: 40 })} />);
    expect(getByRole('progressbar')).toHaveAttribute('aria-valuenow', '40');
    rerender(<JobStatusBanner job={job('failed', { message: 'Tarife bulunamadı' })} />);
    await expect.poll(() => baseElement.querySelector('[aria-live="assertive"]')?.textContent).toBe('Ağustos faturaları: Başarısız');
    expect(getByRole('alert')).toHaveTextContent('Tarife bulunamadı');
  });

  it('success offers the result action and dismiss', async () => {
    const onClick = vi.fn();
    const onDismiss = vi.fn();
    const { getByRole, user } = renderWithProviders(
      <JobStatusBanner job={job('succeeded', { resultAction: { label: 'Faturaları aç', onClick } })} onDismiss={onDismiss} />,
    );
    await user.click(getByRole('button', { name: 'Faturaları aç' }));
    await user.click(getByRole('button', { name: 'Kapat' }));
    expect(onClick).toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalled();
  });

  it('no job renders nothing', () => {
    const { container } = renderWithProviders(<JobStatusBanner job={null} />);
    expect(container.querySelector('[role="status"], [role="alert"]')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<JobStatusBanner job={job('running', { progress: null })} />);
    await expectNoAxeViolations(container);
  });
});
