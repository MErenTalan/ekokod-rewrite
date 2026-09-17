import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { FileList } from './file-list';

const files = [
  { id: '1', name: 'fatura.pdf', sizeBytes: 1_500_000, status: 'done' as const },
  { id: '2', name: 'tarife.xlsx', sizeBytes: 20_000, status: 'uploading' as const, progress: 40 },
  { id: '3', name: 'icmal.csv', sizeBytes: 900, status: 'error' as const, error: 'Sunucu hatası' },
];

describe('FileList', () => {
  it('remove button is labelled with the file name', async () => {
    const onRemove = vi.fn();
    const { getByRole, user } = renderWithProviders(<FileList files={files} onRemove={onRemove} />);
    await user.click(getByRole('button', { name: /fatura\.pdf/ }));
    expect(onRemove).toHaveBeenCalledWith('1');
  });

  it('uploading rows have a named progress bar and errors are text', () => {
    const { getByRole, getByText } = renderWithProviders(<FileList files={files} onRemove={() => {}} />);
    expect(getByRole('progressbar', { name: 'tarife.xlsx yükleniyor' })).toHaveAttribute('aria-valuenow', '40');
    expect(getByText('Sunucu hatası')).toBeInTheDocument();
    expect(getByText('1,5 MB')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<FileList files={files} onRemove={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
