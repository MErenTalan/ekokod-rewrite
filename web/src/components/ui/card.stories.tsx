import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Button } from './button';
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from './card';

const meta = { title: 'UI/Card', component: Card } satisfies Meta<typeof Card>;
export default meta;
type Story = StoryObj<typeof meta>;

const Example = ({ title = 'Son fatura' }: { title?: string }) => (
  <Card className="max-w-sm">
    <CardHeader>
      <CardTitle>{title}</CardTitle>
      <CardDescription>Merkez Bina · Ağustos 2026</CardDescription>
    </CardHeader>
    <CardContent>
      <p className="type-metric">₺184.230,45</p>
    </CardContent>
    <CardFooter>
      <Button variant="secondary" size="sm">
        Faturayı aç
      </Button>
    </CardFooter>
  </Card>
);

export const Default: Story = { render: () => <Example /> };
export const ShadowMode: Story = {
  render: () => (
    <div data-card="shadow">
      <Example />
    </div>
  ),
};
export const LongTurkishLabel: Story = { render: () => <Example title="Reaktif Endüktif Tüketim Oranı Eşik Değeri Aşımı" /> };
