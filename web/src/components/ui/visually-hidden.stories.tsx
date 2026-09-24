import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { X } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { VisuallyHidden } from './visually-hidden';

const meta = { title: 'UI/VisuallyHidden', component: VisuallyHidden } satisfies Meta<typeof VisuallyHidden>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return (
      <button type="button" className="inline-flex size-9 items-center justify-center rounded-md text-foreground hover:bg-surface-sunken pointer-coarse:size-11">
        <X aria-hidden className="size-4" />
        <VisuallyHidden>{t('close')}</VisuallyHidden>
      </button>
    );
  },
};
