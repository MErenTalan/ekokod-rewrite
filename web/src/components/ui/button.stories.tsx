import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { ArrowRight, Download, Save } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from './button';

const meta = { title: 'UI/Button', component: Button, args: { children: '' } } satisfies Meta<typeof Button>;
export default meta;
type Story = StoryObj<typeof meta>;

const variants = ['primary', 'secondary', 'ghost', 'danger'] as const;

export const Variants: Story = {
  render: function Render() {
    const t = useTranslations('common');
    const labels = { primary: t('save'), secondary: t('cancel'), ghost: t('edit'), danger: t('delete') };
    return (
      <div className="flex flex-wrap gap-3">
        {variants.map((v) => (
          <Button key={v} variant={v}>
            {labels[v]}
          </Button>
        ))}
      </div>
    );
  },
};

export const Sizes: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return (
      <div className="flex flex-wrap items-center gap-3">
        <Button size="sm">{t('apply')}</Button>
        <Button size="md">{t('apply')}</Button>
        <Button size="lg">{t('apply')}</Button>
      </div>
    );
  },
};

export const Loading: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return <Button loading>{t('save')}</Button>;
  },
};

export const Disabled: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return (
      <div className="flex flex-wrap gap-3">
        {variants.map((v) => (
          <Button key={v} variant={v} disabled>
            {t('save')}
          </Button>
        ))}
      </div>
    );
  },
};

export const WithIcons: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return (
      <div className="flex flex-wrap gap-3">
        <Button iconStart={Save}>{t('save')}</Button>
        <Button variant="secondary" iconStart={Download}>
          CSV
        </Button>
        <Button variant="ghost" iconEnd={ArrowRight}>
          {t('more')}
        </Button>
      </div>
    );
  },
};

export const AsLink: Story = {
  render: () => (
    <Button asChild variant="secondary">
      <a href="#bills">Faturalar</a>
    </Button>
  ),
};

export const LongTurkishLabel: Story = {
  render: () => (
    <div className="max-w-xs">
      <Button iconStart={Save}>Reaktif Endüktif Tüketim Oranı Eşik Değeri</Button>
    </div>
  ),
};
