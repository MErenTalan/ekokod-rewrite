import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { FileUpload } from './file-upload';

const file = (name: string, size: number, type: string) => new File([new Uint8Array(size)], name, { type });

describe('FileUpload', () => {
  it('rejects oversize files with a visible error', async () => {
    const onFilesSelected = vi.fn();
    const { getByLabelText, getByText, user } = renderWithProviders(
      <FileUpload label="Fatura dosyası" accept=".pdf,application/pdf" maxSizeBytes={1_000_000} onFilesSelected={onFilesSelected} />,
    );
    await user.upload(getByLabelText('Fatura dosyası'), file('fatura.pdf', 2_000_000, 'application/pdf'));
    expect(onFilesSelected).not.toHaveBeenCalled();
    expect(getByText('fatura.pdf en fazla 1 MB olabilir')).toBeVisible();
  });

  it('rejects a wrong type and accepts a valid file', async () => {
    const onFilesSelected = vi.fn();
    const { getByLabelText, getByText, queryByText } = renderWithProviders(
      <FileUpload label="Tarife" accept=".xlsx,.csv" maxSizeBytes={1_000_000} onFilesSelected={onFilesSelected} />,
    );
    const input = getByLabelText('Tarife');
    // A drop bypasses the picker's accept filter, so the component must check the type itself.
    const user = userEvent.setup({ applyAccept: false });
    await user.upload(input, file('tarife.exe', 10, 'application/octet-stream'));
    expect(getByText('tarife.exe desteklenmeyen dosya türü')).toBeInTheDocument();
    const ok = file('tarife.csv', 10, 'text/csv');
    await user.upload(input, ok);
    expect(onFilesSelected).toHaveBeenCalledWith([ok]);
    expect(queryByText('tarife.exe desteklenmeyen dosya türü')).not.toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<FileUpload label="Dosya" accept=".pdf" maxSizeBytes={1000} onFilesSelected={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
