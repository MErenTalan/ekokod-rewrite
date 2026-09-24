import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ExportMenu } from './export-menu';

describe('ExportMenu', () => {
  it('each format calls onExport', async () => {
    const onExport = vi.fn();
    const { getByRole, findByRole, user } = renderWithProviders(<ExportMenu onExport={onExport} />);
    await user.click(getByRole('button', { name: 'Dışa aktar' }));
    await user.click(await findByRole('menuitem', { name: 'Excel' }));
    expect(onExport).toHaveBeenCalledWith('excel');
  });

  it('while busy the menu does not open and the start is announced', async () => {
    const { getByRole, queryByRole, user, baseElement } = renderWithProviders(<ExportMenu onExport={() => {}} busyFormat="pdf" />);
    const trigger = getByRole('button', { name: /Dışa aktar/ });
    expect(trigger).toHaveAttribute('aria-busy', 'true');
    await user.click(trigger);
    expect(queryByRole('menu')).not.toBeInTheDocument();
    await expect.poll(() => baseElement.querySelector('[aria-live="polite"]')?.textContent).toBe('PDF dışa aktarımı başladı');
  });

  it('only the requested formats are listed', async () => {
    const { getByRole, findAllByRole, user } = renderWithProviders(<ExportMenu formats={['csv', 'pdf']} onExport={() => {}} />);
    await user.click(getByRole('button', { name: 'Dışa aktar' }));
    expect((await findAllByRole('menuitem')).map((i) => i.textContent)).toEqual(['CSV', 'PDF']);
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<ExportMenu onExport={() => {}} />);
    await expectNoAxeViolations(container);
  });
});
