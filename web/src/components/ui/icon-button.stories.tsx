import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { Bell, Download, Settings2, Trash2 } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { IconButton } from './icon-button';

const meta = { title: 'UI/IconButton', component: IconButton, args: { label: '', icon: Bell } } satisfies Meta<typeof IconButton>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return (
      <div className="flex gap-3">
        <IconButton label={t('notifications')} icon={Bell} />
        <IconButton label={t('actions')} icon={Settings2} variant="secondary" />
        <IconButton label="CSV" icon={Download} variant="primary" size="lg" />
        <IconButton label={t('delete')} icon={Trash2} variant="danger" size="sm" />
      </div>
    );
  },
};

export const Disabled: Story = {
  render: function Render() {
    const t = useTranslations('common');
    return <IconButton label={t('delete')} icon={Trash2} disabled />;
  },
};
