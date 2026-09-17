import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { FileUpload } from './file-upload';

const meta = {
  title: 'UI/FileUpload',
  component: FileUpload,
  args: { label: 'Tarife dosyası', accept: '.xlsx,.csv', maxSizeBytes: 5_000_000, onFilesSelected: () => {} },
} satisfies Meta<typeof FileUpload>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { description: 'EPDK tarife tablosunu yükleyin' } };
export const Error: Story = { args: { error: 'tarife.pdf desteklenmeyen dosya türü' } };
export const Disabled: Story = { args: { disabled: true } };
export const LongTurkishLabel: Story = { args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri tablosu' } };
