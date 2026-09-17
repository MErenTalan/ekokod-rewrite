import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Pagination } from './pagination';

describe('Pagination', () => {
  it('announces page x of y and disables at bounds', async () => {
    const onPageChange = vi.fn();
    const { getByRole, getByText, rerender, user } = renderWithProviders(<Pagination page={1} pageCount={5} onPageChange={onPageChange} />);
    expect(getByText('Sayfa 1 / 5')).toBeInTheDocument();
    expect(getByRole('button', { name: 'Önceki sayfa' })).toBeDisabled();
    await user.click(getByRole('button', { name: 'Sonraki sayfa' }));
    expect(onPageChange).toHaveBeenCalledWith(2);
    rerender(<Pagination page={5} pageCount={5} onPageChange={onPageChange} />);
    expect(getByRole('button', { name: 'Sonraki sayfa' })).toBeDisabled();
  });

  it('page size is a labelled native select', async () => {
    const onPageSizeChange = vi.fn();
    const { getByLabelText, getByText, user } = renderWithProviders(
      <Pagination page={1} pageCount={2} onPageChange={() => {}} pageSize={25} pageSizeOptions={[25, 50]} onPageSizeChange={onPageSizeChange} totalItems={1234} />,
    );
    await user.selectOptions(getByLabelText('Sayfa başına satır'), '50');
    expect(onPageSizeChange).toHaveBeenCalledWith(50);
    expect(getByText('1.234 kayıt')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Pagination page={2} pageCount={3} onPageChange={() => {}} pageSize={25} pageSizeOptions={[25, 50]} onPageSizeChange={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
