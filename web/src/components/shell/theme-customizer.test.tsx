import { afterEach, describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ThemeCustomizer } from './theme-customizer';

describe('ThemeCustomizer', () => {
  afterEach(() => {
    document.cookie = 'ekokod_ui=; max-age=0; path=/';
    document.documentElement.removeAttribute('data-theme');
    document.documentElement.style.removeProperty('--radius-scale');
  });

  it('changing theme updates html and cookie', async () => {
    const { findByRole, user } = renderWithProviders(<ThemeCustomizer open onOpenChange={() => {}} />);
    await user.click(await findByRole('radio', { name: 'Karanlık' }));
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.cookie).toContain('ekokod_ui=');
    expect(document.cookie).toContain('%22dark%22');
  });

  it('radius slider sets --radius-scale', async () => {
    const { findByRole, user } = renderWithProviders(<ThemeCustomizer open onOpenChange={() => {}} />);
    (await findByRole('slider', { name: 'Tema Kenar Yuvarlaklığı' })).focus();
    await user.keyboard('{ArrowRight}');
    expect(document.documentElement.style.getPropertyValue('--radius-scale')).toBe('1.25');
  });

  it('reset restores defaults', async () => {
    const { findByRole, getByRole, user } = renderWithProviders(<ThemeCustomizer open onOpenChange={() => {}} />, { preferences: { layout: 'horizontal' } });
    await user.click(await findByRole('button', { name: 'Varsayılana dön' }));
    expect(getByRole('radio', { name: 'Dikey' })).toBeChecked();
  });

  it('has no axe violations', async () => {
    const { baseElement, findByRole } = renderWithProviders(<ThemeCustomizer open onOpenChange={() => {}} />);
    await findByRole('dialog');
    await expectNoAxeViolations(baseElement);
  });
});
