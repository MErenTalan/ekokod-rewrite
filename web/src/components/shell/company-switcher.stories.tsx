import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { CompanySwitcherView } from './company-switcher';

const meta = {
  title: 'Shell/CompanySwitcher',
  component: CompanySwitcherView,
  args: {
    own: { id: 'p', name: 'EKOKOD Platform' },
    companies: [
      { id: 'a', name: 'Anadolu Tekstil A.Ş.' },
      { id: 'b', name: 'Boğaziçi Gıda San. ve Tic. Ltd. Şti.' },
    ],
    value: undefined,
    onValueChange: () => {},
  },
} satisfies Meta<typeof CompanySwitcherView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const OwnCompany: Story = {};
export const OtherCompany: Story = { args: { value: 'b' } };
