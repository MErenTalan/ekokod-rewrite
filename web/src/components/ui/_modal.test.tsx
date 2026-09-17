import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Modal } from './_modal';

describe('Modal', () => {
  it('renders title, description, body, footer and a labelled close button', async () => {
    const { findByRole, getByRole, getByText, baseElement } = renderWithProviders(
      <Modal open onOpenChange={() => {}} title="Başlık" description="Açıklama" footer={<span>Alt</span>} className="inset-0">
        Gövde
      </Modal>,
    );
    const dialog = await findByRole('dialog', { name: 'Başlık' });
    expect(dialog).toHaveAccessibleDescription('Açıklama');
    expect(getByText('Gövde')).toBeInTheDocument();
    expect(getByText('Alt')).toBeInTheDocument();
    expect(getByRole('button', { name: 'Kapat' })).toBeInTheDocument();
    await expectNoAxeViolations(baseElement);
  });
});
