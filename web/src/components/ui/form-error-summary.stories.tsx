import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import { useTranslations } from 'next-intl';

import { FormErrorSummary } from './form-error-summary';
import { Input } from './input';

function Example() {
  const t = useTranslations('forms');
  return (
    <div className="flex max-w-md flex-col gap-4">
      <FormErrorSummary
        title={t('errorSummaryTitle')}
        errors={[
          { fieldId: 'tariff-name', message: 'Tarife adı zorunludur' },
          { fieldId: 'unit-price', message: 'Birim fiyat 0’dan büyük olmalı' },
        ]}
      />
      <Input id="tariff-name" label="Tarife adı" error="Tarife adı zorunludur" />
      <Input id="unit-price" label="Birim fiyat" error="Birim fiyat 0’dan büyük olmalı" />
    </div>
  );
}

const meta = { title: 'UI/FormErrorSummary', component: FormErrorSummary, args: { errors: [], title: '' } } satisfies Meta<typeof FormErrorSummary>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { render: () => <Example /> };
