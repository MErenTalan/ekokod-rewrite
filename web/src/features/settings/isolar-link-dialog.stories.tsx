import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { IsolarLinkView } from './isolar-link-dialog';

const meta = {
  title: 'Features/Settings/IsolarLink',
  component: IsolarLinkView,
  args: {
    plantId: 'p-1',
    credentials: [{ value: 'c-1', label: 'iSolarCloud (EU)' }],
    credentialId: 'c-1',
    onCredentialChange: () => {},
    onLink: () => {},
    plants: [
      { ps_id: 'PS-1', name: 'Konya Solar', installed_kw: '250' },
      { ps_id: 'PS-2', name: 'Başka Santral', installed_kw: '100', linked_plant_id: 'p-other' },
    ],
  },
} satisfies Meta<typeof IsolarLinkView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const NoCredential: Story = { args: { credentials: [], credentialId: null, plants: [] } };
export const EmptyAccount: Story = { args: { plants: [] } };
