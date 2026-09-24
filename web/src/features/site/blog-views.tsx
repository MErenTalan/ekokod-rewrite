import { ArrowLeft } from 'lucide-react';
import Image from 'next/image';
import Link from 'next/link';
import { useLocale, useTranslations } from 'next-intl';

import { Badge } from '@/components/ui/badge';
import type { Locale } from '@/i18n/locale';
import { formatDate } from '@/lib/format';
import type { BlogCategory, TocEntry } from '@/lib/site/blog';
import { cn } from '@/lib/cn';

import { PageHero } from './site-blocks';

export type BlogCard = {
  slug: string;
  title: string;
  category: BlogCategory;
  date: string;
  author: string;
  description: string;
  cover: { src: string; width: number; height: number };
};
export type BlogFilter = BlogCategory | 'all';
const FILTERS: BlogFilter[] = ['all', 'electricity', 'naturalGas'];

/** Blog listing (01 §7.19): category filter (all / electricity / natural gas) and article cards. */
export function BlogList({ posts, category }: { posts: BlogCard[]; category: BlogFilter }) {
  const t = useTranslations('site.blog');
  const locale = useLocale() as Locale;
  return (
    <>
      <PageHero title={t('title')} description={t('description')} />
      <div className="mx-auto flex max-w-7xl flex-col gap-8 px-4 py-12 sm:px-6">
        <nav aria-label={t('filter')}>
          <ul className="flex flex-wrap gap-2">
            {FILTERS.map((f) => (
              <li key={f}>
                <Link
                  href={f === 'all' ? '/blog' : `/blog?category=${f}`}
                  aria-current={f === category ? 'page' : undefined}
                  className={cn(
                    'inline-flex min-h-9 items-center rounded-full border px-4 font-medium pointer-coarse:min-h-11',
                    f === category ? 'border-primary bg-primary text-on-primary' : 'border-border-control text-foreground hover:bg-surface-sunken',
                  )}
                >
                  {t(`categories.${f}`)}
                </Link>
              </li>
            ))}
          </ul>
        </nav>
        {posts.length === 0 ? (
          <p className="text-foreground-muted type-body-lg">{t('empty')}</p>
        ) : (
          <ul className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {posts.map((p) => (
              <li key={p.slug}>
                <article className="relative flex h-full flex-col overflow-hidden rounded-lg border border-border bg-surface">
                  <Image src={p.cover.src} alt="" width={p.cover.width} height={p.cover.height} sizes="(min-width: 1024px) 400px, (min-width: 768px) 50vw, 100vw" className="aspect-[2/1] w-full object-cover" />
                  <div className="flex flex-1 flex-col gap-2 p-5">
                    <div className="flex flex-wrap items-center gap-2 text-foreground-muted type-small">
                      <Badge tone="success">{t(`categories.${p.category}`)}</Badge>
                      <time dateTime={p.date}>{formatDate(p.date, locale)}</time>
                    </div>
                    <h2 className="text-foreground type-h2">
                      <Link href={`/blog/${p.slug}`} className="after:absolute after:inset-0 hover:underline">{p.title}</Link>
                    </h2>
                    <p className="flex-1 text-foreground-muted">{p.description}</p>
                    <p className="text-foreground-muted type-small">{t('by', { author: p.author })}</p>
                  </div>
                </article>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  );
}

export type BlogPost = BlogCard & { html: string; toc: TocEntry[] };

/** One article: title, meta line, cover (the page's LCP image), contents and the rendered Markdown. */
export function BlogArticle({ post }: { post: BlogPost }) {
  const t = useTranslations('site.blog');
  const locale = useLocale() as Locale;
  return (
    <article className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-10 sm:px-6" lang="tr">
      <Link href="/blog" className="inline-flex min-h-11 items-center gap-1 self-start font-semibold text-primary hover:underline" lang={locale}>
        <ArrowLeft aria-hidden className="size-4" />
        {t('back')}
      </Link>
      <header className="flex flex-col gap-3">
        <h1 className="text-foreground type-display md:text-[40px] md:leading-[48px]">{post.title}</h1>
        <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-foreground-muted" lang={locale}>
          <Badge tone="success">{t(`categories.${post.category}`)}</Badge>
          <time dateTime={post.date}>{t('published', { date: formatDate(post.date, locale) })}</time>
          <span>{t('by', { author: post.author })}</span>
        </p>
        {locale !== 'tr' ? <p className="text-foreground-muted type-small" lang={locale}>{t('turkishOnly')}</p> : null}
      </header>
      <Image src={post.cover.src} alt="" width={post.cover.width} height={post.cover.height} priority sizes="(min-width: 768px) 720px, 100vw" className="h-auto w-full rounded-lg" />
      {post.toc.length > 0 ? (
        <nav aria-label={t('contents')} className="rounded-lg border border-border bg-surface-sunken p-5" lang={locale}>
          <p className="mb-2 text-foreground type-h3" aria-hidden>{t('contents')}</p>
          <ol className="flex flex-col gap-1" lang="tr">
            {post.toc.map((e) => (
              <li key={e.id} className={e.depth === 3 ? 'ps-4' : undefined}>
                <a href={`#${e.id}`} className="inline-flex min-h-8 items-center text-primary hover:underline pointer-coarse:min-h-11">{e.text}</a>
              </li>
            ))}
          </ol>
        </nav>
      ) : null}
      {/* Repo-owned Markdown rendered by lib/site/blog (Q-H7): trusted content, raw HTML allowed. */}
      <div className="site-prose" dangerouslySetInnerHTML={{ __html: post.html }} />
    </article>
  );
}
