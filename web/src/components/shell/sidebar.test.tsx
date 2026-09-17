import { within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { setMockPathname } from '@/test/navigation';
import { renderWithProviders } from '@/test/render';

import { Sidebar } from './sidebar';

const topLevel = (nav: HTMLElement) =>
  [...nav.querySelectorAll(':scope > ul > li > :is(a, button, [role="link"])')].map((el) => el.textContent?.trim());

describe('Sidebar', () => {
  afterEach(() => setMockPathname('/'));

  it('groups and leaves match 07 §7 order', () => {
    const { getByRole } = renderWithProviders(<Sidebar collapsed={false} />);
    expect(topLevel(getByRole('navigation', { name: 'Ana menü' }))).toEqual([
      'Yönetim Arayüzü',
      'Veri Analizi',
      'Faturalar ve Tarifeler',
      'Alarmlar',
      'Raporlar',
      'Ayarlar',
      'Karbon Ayak İzi',
      'ISO 50001 Modülü',
      'Tasarruf Önerileri',
      'İletişim',
    ]);
  });

  it('disabled entries are visible, not links, and explain why', async () => {
    setMockPathname('/consumption');
    const { getByRole, findByRole, user } = renderWithProviders(<Sidebar collapsed={false} />);
    await user.click(getByRole('button', { name: 'Alarmlar' }));
    for (const name of ['Su', 'Doğal Gaz', 'EV Sürücüleri', 'Yapay Zeka', 'Tasarruf Önerileri']) {
      const entry = getByRole('link', { name });
      expect(entry).toHaveAttribute('aria-disabled', 'true');
      expect(entry).not.toHaveAttribute('href');
    }
    getByRole('link', { name: 'Su' }).focus();
    expect(await findByRole('tooltip')).toHaveTextContent('Bu bölüm henüz kullanıma açılmadı');
  });

  it('active route is marked', () => {
    setMockPathname('/load-profile');
    const { getByRole } = renderWithProviders(<Sidebar collapsed={false} />);
    expect(getByRole('link', { name: 'Yük Profili' })).toHaveAttribute('aria-current', 'page');
    expect(getByRole('button', { name: 'Veri Analizi' })).toHaveAttribute('aria-expanded', 'true');
    expect(getByRole('button', { name: 'Alarmlar' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('only the most specific entry is active', async () => {
    setMockPathname('/alarms/ai');
    const { queryAllByRole } = renderWithProviders(<Sidebar collapsed={false} />);
    expect(queryAllByRole('link', { current: 'page' })).toHaveLength(0);
    setMockPathname('/alarms');
    const second = renderWithProviders(<Sidebar collapsed={false} />);
    expect(second.getAllByRole('link', { current: 'page' }).map((l) => l.textContent)).toEqual(['Manuel']);
  });

  it('navigating calls onNavigate (closes the drawer)', async () => {
    const onNavigate = vi.fn();
    const { getByRole, user } = renderWithProviders(<Sidebar collapsed={false} onNavigate={onNavigate} />);
    const link = getByRole('link', { name: 'Raporlar' });
    link.addEventListener('click', (e) => e.preventDefault());
    await user.click(link);
    expect(onNavigate).toHaveBeenCalled();
  });

  it('collapsed shows icon entries named by their label', () => {
    const { getByRole } = renderWithProviders(<Sidebar collapsed />);
    const nav = getByRole('navigation', { name: 'Ana menü' });
    expect(within(nav).getByRole('link', { name: 'Raporlar' })).toBeInTheDocument();
    expect(within(nav).getByRole('button', { name: 'Veri Analizi' })).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    setMockPathname('/bills');
    const { container } = renderWithProviders(<Sidebar collapsed={false} />);
    await expectNoAxeViolations(container);
  });
});
