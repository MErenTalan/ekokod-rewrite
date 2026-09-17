import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { controlClasses, Field, type FieldIds, type FieldProps } from './field';

const input = ({ controlId, describedBy, invalid }: FieldIds) => (
  <input id={controlId} aria-describedby={describedBy} aria-invalid={invalid || undefined} className={controlClasses} defaultValue="enerji@ornek.com.tr" />
);
const Example = (props: FieldProps) => <Field {...props}>{input}</Field>;

const meta = { title: 'UI/Field', component: Example, args: { label: 'E-posta' } } satisfies Meta<typeof Example>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { args: { description: 'Bildirimler bu adrese gönderilir' } };
export const Error: Story = { args: { required: true, error: 'Geçerli bir e-posta adresi girin' } };
export const LongTurkishLabel: Story = {
  args: { label: 'Reaktif Endüktif Tüketim Oranı Eşik Değeri', description: 'Dağıtım şirketinin uyguladığı yüzde sınırı' },
};
