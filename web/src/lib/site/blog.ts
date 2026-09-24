import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { Marked, Renderer, type Tokens } from 'marked';

// 01 §7.19 blog: repo-owned Markdown (GFM + raw HTML; Q-H7). The content is trusted —
// it ships with the code and is reviewed like code — so raw HTML is not sanitised.

export type BlogCategory = 'electricity' | 'naturalGas';
export type TocEntry = { id: string; text: string; depth: number };
export type PostMeta = {
  slug: string;
  title: string;
  file: string;
  category: BlogCategory;
  date: string;
  author: string;
  cover: { src: string; width: number; height: number };
};

const LIST: PostMeta[] = [
  {
    slug: 'elektrik-faturam-neden-yuksek-1',
    title: 'Elektrik Faturam Neden Yüksek?',
    file: 'elektrik-faturam-neden-yuksek-1.md',
    category: 'electricity',
    date: '2026-08-05',
    author: 'Metin Ağan',
    cover: { src: '/site/blog/elektrik-faturam-img0.webp', width: 1600, height: 768 },
  },
  {
    slug: 'elektrik-faturam-neden-yuksek-2',
    title: 'Elektrik Faturam Neden Yüksek? Yazı-2',
    file: 'elektrik-faturam-neden-yuksek-2.md',
    category: 'electricity',
    date: '2026-09-02',
    author: 'Metin Ağan',
    cover: { src: '/site/blog/elektrik-faturam-2-img0.webp', width: 1600, height: 768 },
  },
];

/** Newest first. */
export const POSTS: PostMeta[] = [...LIST].sort((a, b) => b.date.localeCompare(a.date));

export function postsIn(category: BlogCategory | 'all'): PostMeta[] {
  return POSTS.filter((p) => category === 'all' || p.category === category);
}

const TR: Record<string, string> = { ç: 'c', ğ: 'g', ı: 'i', i̇: 'i', ö: 'o', ş: 's', ü: 'u' };

export function slugify(text: string): string {
  return text
    .toLocaleLowerCase('tr')
    .replace(/i̇|[çğıöşü]/g, (c) => TR[c] ?? c)
    .normalize('NFKD')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');

export function renderMarkdown(markdown: string): { html: string; toc: TocEntry[] } {
  const toc: TocEntry[] = [];
  const seen = new Map<string, number>();
  const marked = new Marked({
    gfm: true,
    renderer: {
      heading({ tokens, depth, text }: Tokens.Heading) {
        const plain = text.replace(/<[^>]+>/g, '').replace(/[*_`]/g, '').trim();
        const base = slugify(plain) || 'bolum';
        const n = (seen.get(base) ?? 0) + 1;
        seen.set(base, n);
        const id = n === 1 ? base : `${base}-${n}`;
        if (depth === 2 || depth === 3) toc.push({ id, text: plain, depth });
        return `<h${depth} id="${id}">${this.parser.parseInline(tokens)}</h${depth}>\n`;
      },
      image({ href, title, text }: Tokens.Image) {
        const t = title ? ` title="${escapeAttr(title)}"` : '';
        return `<img src="${escapeAttr(href)}" alt="${escapeAttr(text)}"${t} loading="lazy" decoding="async">`;
      },
      table(token: Tokens.Table) {
        // A focusable wrapper keeps wide tables scrollable by keyboard (WCAG 2.1.1).
        const inner = Renderer.prototype.table.call(this, token);
        return `<div class="table-scroll" tabindex="0" role="region" aria-label="Tablo">${inner}</div>\n`;
      },
    },
  });
  const html = marked.parse(markdown, { async: false });
  return { html, toc };
}

/** First paragraph as plain text, cut at `max` characters. */
export function excerpt(markdown: string, max = 160): string {
  const para = markdown.split(/\n{2,}/).map((p) => p.trim()).find((p) => p && !/^(#|<|!\[|\||-|\d+\.)/.test(p)) ?? '';
  const plain = para.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[*_`]/g, '').replace(/\s+/g, ' ').trim();
  return plain.length <= max ? plain : `${plain.slice(0, max).trimEnd()}…`;
}

const cache = new Map<string, { meta: PostMeta; html: string; toc: TocEntry[]; description: string }>();

/** Only listed slugs are read; the slug never becomes part of a path. */
export function readPost(slug: string) {
  const meta = LIST.find((p) => p.slug === slug);
  if (!meta) return null;
  const hit = cache.get(slug);
  if (hit) return hit;
  const markdown = readFileSync(join(process.cwd(), 'content', 'blog', meta.file), 'utf8');
  const post = { meta, ...renderMarkdown(markdown), description: excerpt(markdown) };
  cache.set(slug, post);
  return post;
}
