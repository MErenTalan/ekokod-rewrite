import { Save } from 'lucide-react';
import { describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { messages } from '../../../messages';
import { Button } from './button';

describe('Button', () => {
  it('renders a native button with type=button', () => {
    const { getByRole } = renderWithProviders(<Button>Kaydet</Button>);
    expect(getByRole('button', { name: 'Kaydet' })).toHaveAttribute('type', 'button');
  });

  it('loading keeps focus, is busy and swallows clicks', async () => {
    const onClick = vi.fn();
    const { getByRole, getByText, user } = renderWithProviders(
      <Button loading onClick={onClick}>
        Kaydet
      </Button>,
    );
    const button = getByRole('button', { name: /Kaydet/ });
    button.focus();
    await user.click(button);
    expect(onClick).not.toHaveBeenCalled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button).toHaveAttribute('aria-disabled', 'true');
    expect(button).toHaveFocus();
    expect(getByText(messages.tr.common.loading)).toBeInTheDocument();
  });

  it('disabled does not fire onClick', async () => {
    const onClick = vi.fn();
    const { getByRole, user } = renderWithProviders(
      <Button disabled onClick={onClick}>
        Kaydet
      </Button>,
    );
    await user.click(getByRole('button'));
    expect(onClick).not.toHaveBeenCalled();
  });

  it('icons are decorative', () => {
    const { container } = renderWithProviders(
      <Button iconStart={Save} iconEnd={Save}>
        Kaydet
      </Button>,
    );
    const svgs = container.querySelectorAll('svg');
    expect(svgs).toHaveLength(2);
    svgs.forEach((svg) => expect(svg).toHaveAttribute('aria-hidden', 'true'));
  });

  it('asChild renders the link with button styling', () => {
    const { getByRole } = renderWithProviders(
      <Button asChild>
        <a href="/bills">Faturalar</a>
      </Button>,
    );
    expect(getByRole('link').className).toContain('bg-primary');
  });

  it('every variant and size has no axe violations', async () => {
    const { container } = renderWithProviders(
      <div>
        {(['primary', 'secondary', 'ghost', 'danger'] as const).flatMap((variant) =>
          (['sm', 'md', 'lg'] as const).map((size) => (
            <Button key={`${variant}-${size}`} variant={variant} size={size}>
              {`${variant} ${size}`}
            </Button>
          )),
        )}
      </div>,
    );
    await expectNoAxeViolations(container);
  });
});
