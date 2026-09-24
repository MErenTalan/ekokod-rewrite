import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { fn } from 'storybook/test';

import { file } from './_fixture';
import { FilesView } from './files-view';

const meta = {
  title: 'Features/Iso50001/Files',
  component: FilesView,
  args: { files: [file(), file({ id: 'f-2', name: 'Enerji-Politikasi.docx', size_bytes: 88_000 })], editable: true, uploading: false, onUpload: fn(), onDownload: fn(), onDelete: fn() },
} satisfies Meta<typeof FilesView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Files: Story = {};
export const Empty: Story = { args: { files: [] } };
export const Refused: Story = { args: { error: 'Dosya boyutu 30 MB sınırını aşamaz.' } };
export const ReadOnly: Story = { args: { editable: false } };
