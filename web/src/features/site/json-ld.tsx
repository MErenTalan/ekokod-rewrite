import { jsonLd } from '@/lib/site/structured-data';

/** Structured data for search engines (09 §F12 SEO). */
export function JsonLd({ data }: { data: unknown }) {
  // jsonLd escapes `<`, so the payload cannot leave the script element.
  return <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLd(data) }} />;
}
