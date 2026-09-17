import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Button } from './button';
import { useAnnounce } from './live-announcer';

function Example() {
  const announce = useAnnounce();
  return (
    <div className="flex flex-wrap gap-2">
      <Button variant="secondary" onClick={() => announce('Rapor hazır')}>
        Nazik duyuru
      </Button>
      <Button variant="danger" onClick={() => announce('Alarm: Merkez Bina reaktif sınırı aştı', 'assertive')}>
        Acil duyuru
      </Button>
    </div>
  );
}

const meta = { title: 'UI/LiveAnnouncer', component: Example } satisfies Meta<typeof Example>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
