import { afterEach, describe, expect, it } from 'vitest';

import { mockRouter } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { ModuleSwitcher } from './module-switcher';

afterEach(() => mockRouter.push.mockClear());

describe('ModuleSwitcher (F17 R496)', () => {
  it('moves to the other module on the same session', async () => {
    const r = renderWithProviders(<ModuleSwitcher current="ekorm" modules={['ekorm', 'ekocm']} />);
    await r.user.click(r.getByRole('button', { name: 'Modül değiştir' }));
    expect(await r.findByRole('menuitem', { name: /EKO-RM · Kaynak Yönetimi/ })).toHaveAttribute('aria-disabled', 'true');
    await r.user.click(r.getByRole('menuitem', { name: /EKO-CM · Karbon Yönetimi/ }));
    expect(mockRouter.push).toHaveBeenCalledWith('/ekocm');
  });

  it('offers the chooser as well', async () => {
    const r = renderWithProviders(<ModuleSwitcher current="ekocm" modules={['ekorm', 'ekocm']} />);
    await r.user.click(r.getByRole('button', { name: 'Modül değiştir' }));
    await r.user.click(await r.findByRole('menuitem', { name: 'Tüm modüller' }));
    expect(mockRouter.push).toHaveBeenCalledWith('/modules');
  });
});
