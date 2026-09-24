import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { BlogArticle, BlogList, type BlogCard } from './blog-views';

const card = (slug: string, title: string, date: string, cover: string): BlogCard => ({
  slug, title, date, category: 'electricity', author: 'Metin Ağan',
  description: 'Bu yazı serisinde elektrik faturalarını oluşturan kalemlere tek tek bakıyoruz.',
  cover: { src: cover, width: 1600, height: 768 },
});
const posts = [
  card('elektrik-faturam-neden-yuksek-2', 'Elektrik Faturam Neden Yüksek? Yazı-2', '2026-09-02', '/site/blog/elektrik-faturam-2-img0.webp'),
  card('elektrik-faturam-neden-yuksek-1', 'Elektrik Faturam Neden Yüksek?', '2026-08-05', '/site/blog/elektrik-faturam-img0.webp'),
];

const meta = { title: 'Features/Site/Blog', component: BlogList, args: { posts, category: 'all' }, parameters: { layout: 'fullscreen' } } satisfies Meta<typeof BlogList>;
export default meta;
type Story = StoryObj<typeof meta>;

export const List: Story = {};
export const EmptyCategory: Story = { args: { posts: [], category: 'naturalGas' } };
export const Article: Story = {
  render: () => (
    <BlogArticle
      post={{
        ...posts[1],
        toc: [{ id: 'reaktif', text: 'Reaktif Ceza mı ödüyorsunuz?', depth: 2 }, { id: 'tablo', text: 'Tarife tablosu', depth: 3 }],
        html: '<h2 id="reaktif">Reaktif Ceza mı ödüyorsunuz?</h2><p>Reaktif ceza mevzuatta belli limitlerin dışında reaktif güç tüketen abonelere verilen bir cezadır.<sup id="ref-1"><a href="#footnote-1">[1]</a></sup></p><h3 id="tablo">Tarife tablosu</h3><div class="table-scroll" tabindex="0" role="region" aria-label="Tablo"><table><thead><tr><th>Kalem</th><th>Birim</th></tr></thead><tbody><tr><td>Güç bedeli</td><td>kr/kW/ay</td></tr></tbody></table></div><hr><ol class="footnotes"><li id="footnote-1">Madde 16-3. <a href="#ref-1" aria-label="Metne dön (1)">↑</a></li></ol>',
      }}
    />
  ),
};
