import { notFound } from 'next/navigation';

import { BlogArticle } from '@/features/site/blog-views';
import { JsonLd } from '@/features/site/json-ld';
import { readPost } from '@/lib/site/blog';
import { pageMetadata } from '@/lib/site/metadata';
import { siteUrl } from '@/lib/site/seo';

type Params = { params: Promise<{ slug: string }> };

export async function generateMetadata({ params }: Params) {
  const post = readPost((await params).slug);
  if (!post) return {};
  return pageMetadata('blog', `/blog/${post.meta.slug}`, { title: post.meta.title, description: post.description, image: post.meta.cover.src, type: 'article' });
}

export default async function BlogPostPage({ params }: Params) {
  const post = readPost((await params).slug);
  if (!post) notFound();
  const base = siteUrl();
  return (
    <>
      <JsonLd
        data={{
          '@context': 'https://schema.org',
          '@type': 'BlogPosting',
          headline: post.meta.title,
          description: post.description,
          datePublished: post.meta.date,
          inLanguage: 'tr',
          author: { '@type': 'Person', name: post.meta.author },
          image: `${base}${post.meta.cover.src}`,
          mainEntityOfPage: `${base}/blog/${post.meta.slug}`,
          publisher: { '@type': 'Organization', name: 'EkoKod', url: base },
        }}
      />
      <BlogArticle post={{ ...post.meta, description: post.description, html: post.html, toc: post.toc }} />
    </>
  );
}
