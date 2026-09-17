import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Card, CardContent, CardHeader, CardTitle } from './card';
import { StaggerGrid } from './stagger-grid';

const meta = {
  title: 'UI/StaggerGrid',
  component: StaggerGrid,
  args: {
    className: 'sm:grid-cols-2 lg:grid-cols-3',
    children: ['Bina sayısı', 'Aktif analizör', 'Aylık tüketim', 'Reaktif oran'].map((label) => (
      <Card key={label}>
        <CardHeader>
          <CardTitle>{label}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="type-metric">1.234</p>
        </CardContent>
      </Card>
    )),
  },
} satisfies Meta<typeof StaggerGrid>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
