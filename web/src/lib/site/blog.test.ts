// @vitest-environment node
import { existsSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { excerpt, POSTS, postsIn, readPost, renderMarkdown, slugify } from './blog';

describe('renderMarkdown', () => {
  it('gives headings stable Turkish-safe ids and a table of contents', () => {
    const { html, toc } = renderMarkdown('## Güç Faktörü (cosφ)\n\ntext\n\n### Alt başlık\n\n## Güç Faktörü (cosφ)\n');
    expect(toc).toEqual([
      { id: 'guc-faktoru-cos', text: 'Güç Faktörü (cosφ)', depth: 2 },
      { id: 'alt-baslik', text: 'Alt başlık', depth: 3 },
      { id: 'guc-faktoru-cos-2', text: 'Güç Faktörü (cosφ)', depth: 2 },
    ]);
    expect(html).toContain('<h2 id="guc-faktoru-cos">');
    expect(html).toContain('<h3 id="alt-baslik">');
  });

  it('renders GFM tables in a keyboard-scrollable wrapper', () => {
    const { html } = renderMarkdown('| a | b |\n|---|---|\n| 1<br>2 | **3** |\n');
    expect(html).toMatch(/<div class="table-scroll" tabindex="0"[^>]*><table>/);
    expect(html).toContain('<td>1<br>2</td>');
  });

  it('keeps raw HTML and lazy-loads markdown images', () => {
    const { html } = renderMarkdown('x<sup id="ref-1">[\\[1\\]](#footnote-1)</sup>\n\n![Tarife](/site/blog/a.webp)\n');
    expect(html).toContain('<sup id="ref-1"><a href="#footnote-1">[1]</a></sup>');
    expect(html).toContain('<img src="/site/blog/a.webp" alt="Tarife" loading="lazy" decoding="async">');
  });

  it('slugifies Turkish letters', () => {
    expect(slugify('Sözleşme Gücü Neden Aşılmamalı?')).toBe('sozlesme-gucu-neden-asilmamali');
    expect(slugify('İÇİNDEKİLER')).toBe('icindekiler');
  });

  it('excerpts the first paragraph as plain text', () => {
    expect(excerpt('## H\n\nBu **kalın** ve [bağlantılı](#x) bir giriş.\n\nİkinci.', 200)).toBe('Bu kalın ve bağlantılı bir giriş.');
    expect(excerpt('## H\n\n' + 'a '.repeat(200), 20)).toBe('a a a a a a a a a a…');
  });
});

describe('the migrated legacy articles', () => {
  it('both render with a contents list and only images that ship', () => {
    expect(POSTS.map((p) => p.slug)).toEqual(['elektrik-faturam-neden-yuksek-2', 'elektrik-faturam-neden-yuksek-1']);
    for (const meta of POSTS) {
      const post = readPost(meta.slug);
      expect(post?.toc.length).toBeGreaterThan(3);
      const images = [...(post?.html ?? '').matchAll(/<img src="([^"]+)"/g)].map((m) => m[1]);
      for (const src of [...images, meta.cover.src]) expect(existsSync(join(process.cwd(), 'public', src)), src).toBe(true);
    }
    expect(readPost('elektrik-faturam-neden-yuksek-1')?.html).toMatch(/<img src="\/site\/blog\/elektrik-faturam-img\d\.webp"/);
  });

  it('every footnote reference has its target and a way back', () => {
    const html = readPost('elektrik-faturam-neden-yuksek-1')?.html ?? '';
    for (const n of [1, 2, 3]) {
      expect(html).toContain(`id="footnote-${n}"`);
      expect(html).toContain(`href="#ref-${n}"`);
    }
  });

  it('refuses unknown slugs without touching the filesystem path', () => {
    expect(readPost('../../package')).toBeNull();
    expect(readPost('nope')).toBeNull();
  });

  it('filters by category, newest first', () => {
    expect(postsIn('electricity')).toHaveLength(2);
    expect(postsIn('naturalGas')).toEqual([]);
    expect(postsIn('all')[0].date).toBe('2026-09-02');
  });
});
