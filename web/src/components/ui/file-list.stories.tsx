import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { FileList } from './file-list';

const files = [
  { id: '1', name: 'fatura-2026-08.pdf', sizeBytes: 1_540_000, status: 'done' as const },
  { id: '2', name: 'tarife-listesi.xlsx', sizeBytes: 84_000, status: 'uploading' as const, progress: 45 },
  { id: '3', name: 'icmal.csv', sizeBytes: 9_200, status: 'error' as const, error: 'Sunucu dosyayı işleyemedi, tekrar deneyin' },
];

const meta = { title: 'UI/FileList', component: FileList, args: { files, onRemove: () => {} } } satisfies Meta<typeof FileList>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const LongTurkishLabel: Story = {
  args: { files: [{ id: '1', name: 'reaktif-enduktif-tuketim-orani-esik-degeri-raporu-2026-agustos-merkez-bina.pdf', sizeBytes: 2_300_000, status: 'done' }] },
};
