import { BlogList, type BlogFilter } from '@/features/site/blog-views';
import { postsIn, readPost } from '@/lib/site/blog';
import { pageMetadata } from '@/lib/site/metadata';

export const generateMetadata = () => pageMetadata('blog', '/blog');

const FILTERS: BlogFilter[] = ['all', 'electricity', 'naturalGas'];

export default async function BlogPage({ searchParams }: { searchParams: Promise<{ category?: string | string[] }> }) {
  const { category } = await searchParams;
  const filter = FILTERS.find((f) => f === category) ?? 'all';
  const posts = postsIn(filter).map((meta) => ({ ...meta, description: readPost(meta.slug)?.description ?? '' }));
  return <BlogList posts={posts} category={filter} />;
}
