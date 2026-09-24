import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { ContactDetails, DocumentGrid, NewsList, ReferenceGrid } from './site-blocks';

describe('site blocks', () => {
  it('lists the four references with logos and, optionally, their stories', () => {
    const { getAllByRole, getByRole, queryByText, unmount } = renderWithProviders(<ReferenceGrid />);
    expect(getAllByRole('listitem')).toHaveLength(4);
    expect(getByRole('img', { name: 'Hacettepe Üniversitesi logosu' })).toBeInTheDocument();
    expect(queryByText(/Karacabey Hibrit/)).toBeInTheDocument();
    unmount();
    renderWithProviders(<ReferenceGrid stories={false} />);
    expect(queryByText(/Karacabey Hibrit/)).toBeNull();
  });

  it('requests documents through the contact form, except articles which open the blog', () => {
    const { getByRole } = renderWithProviders(<DocumentGrid />);
    const guide = getByRole('link', { name: 'İletişime geçerek isteyin: EkoKod Kullanıcı Kılavuzu' });
    expect(guide.getAttribute('href')).toBe(`/contact?subject=${encodeURIComponent('Doküman talebi: EkoKod Kullanıcı Kılavuzu')}`);
    expect(getByRole('link', { name: 'Blogu aç: Makaleler' })).toHaveAttribute('href', '/blog');
    expect(getByRole('heading', { name: 'Teknik dokümanlar' })).toBeInTheDocument();
  });

  it('opens the news video in a new tab and says so', () => {
    const { getByRole } = renderWithProviders(<NewsList />);
    const video = getByRole('link', { name: /Sunum videosunu izle \(yeni sekmede açılır\)/ });
    expect(video).toHaveAttribute('target', '_blank');
    expect(video).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('gives contact details as working links and a map link, no iframe (Q-H8)', () => {
    const { getByRole, container } = renderWithProviders(<ContactDetails />);
    expect(getByRole('link', { name: '+90 506 315 41 98' })).toHaveAttribute('href', 'tel:+905063154198');
    expect(getByRole('link', { name: /Haritada aç/ }).getAttribute('href')).toMatch(/^https:\/\/www\.openstreetmap\.org\//);
    expect(container.querySelector('iframe')).toBeNull();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(
      <>
        <ReferenceGrid />
        <DocumentGrid level={2} />
        <NewsList />
        <ContactDetails />
      </>,
    );
    await expectNoAxeViolations(container);
  });
});
