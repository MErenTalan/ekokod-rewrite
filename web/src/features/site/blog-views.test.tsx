import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { BlogArticle, BlogList, type BlogCard } from './blog-views';

const card = (slug: string, date: string): BlogCard => ({
  slug, title: `Yazı ${slug}`, category: 'electricity', date, author: 'Metin Ağan', description: 'Giriş paragrafı.',
  cover: { src: '/site/blog/elektrik-faturam-img0.webp', width: 1600, height: 768 },
});

describe('BlogList', () => {
  it('filters by category through links that work without script', () => {
    const { getByRole } = renderWithProviders(<BlogList posts={[card('a', '2026-09-02')]} category="all" />);
    const filter = getByRole('navigation', { name: 'Kategori' });
    expect(filter).toBeInTheDocument();
    expect(getByRole('link', { name: 'Tümü' })).toHaveAttribute('aria-current', 'page');
    expect(getByRole('link', { name: 'Doğal Gaz' })).toHaveAttribute('href', '/blog?category=naturalGas');
    expect(getByRole('link', { name: 'Yazı a' })).toHaveAttribute('href', '/blog/a');
  });

  it('says when a category is empty', () => {
    const { getByText } = renderWithProviders(<BlogList posts={[]} category="naturalGas" />);
    expect(getByText('Bu kategoride henüz yazı yok.')).toBeInTheDocument();
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<BlogList posts={[card('a', '2026-09-02'), card('b', '2026-08-05')]} category="electricity" />);
    await expectNoAxeViolations(container);
  });
});

describe('BlogArticle', () => {
  const post = { ...card('a', '2026-09-02'), html: '<h2 id="giris">Giriş</h2><p>Metin</p>', toc: [{ id: 'giris', text: 'Giriş', depth: 2 }] };

  it('titles the article, links its contents and renders the body', () => {
    const { getByRole, getByText } = renderWithProviders(<BlogArticle post={post} />);
    expect(getByRole('heading', { level: 1, name: 'Yazı a' })).toBeInTheDocument();
    expect(getByRole('navigation', { name: 'İçindekiler' })).toBeInTheDocument();
    expect(getByRole('link', { name: 'Giriş' })).toHaveAttribute('href', '#giris');
    expect(getByRole('heading', { level: 2, name: 'Giriş' })).toBeInTheDocument();
    expect(getByText('Metin')).toBeInTheDocument();
    expect(getByRole('link', { name: /Tüm yazılar/ })).toHaveAttribute('href', '/blog');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<BlogArticle post={post} />);
    await expectNoAxeViolations(container);
  });
});
