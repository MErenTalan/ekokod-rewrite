import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { storyUser } from './_story-user';
import { UserMenu } from './user-menu';

const meta = { title: 'Shell/UserMenu', component: UserMenu, args: { user: storyUser } } satisfies Meta<typeof UserMenu>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LongTurkishLabel: Story = { args: { user: { ...storyUser, name: 'Şükrü Özdemiroğlu Çağlayangil', roleLabel: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri sorumlusu' } } };
