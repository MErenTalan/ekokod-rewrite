import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { JobStatusBanner } from './job-status-banner';

const meta = {
  title: 'Domain/JobStatusBanner',
  component: JobStatusBanner,
  args: { job: { id: 'j1', label: 'Ağustos 2026 faturaları', status: 'running', progress: 45 }, onDismiss: () => {} },
} satisfies Meta<typeof JobStatusBanner>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};
export const Queued: Story = { args: { job: { id: 'j1', label: 'Ağustos 2026 faturaları', status: 'queued' } } };
export const Indeterminate: Story = { args: { job: { id: 'j1', label: 'Tüketim verisi yenileniyor', status: 'running', progress: null } } };
export const Succeeded: Story = { args: { job: { id: 'j1', label: 'Ağustos 2026 faturaları', status: 'succeeded', message: '42 bina için fatura hesaplandı.', resultAction: { label: 'Faturaları aç', onClick: () => {} } } } };
export const Failed: Story = { args: { job: { id: 'j1', label: 'Ağustos 2026 faturaları', status: 'failed', message: '3 bina için geçerli tarife bulunamadı.' } } };
export const LongTurkishLabel: Story = { args: { job: { id: 'j1', label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri yeniden hesaplanıyor', status: 'running', progress: 12 } } };
