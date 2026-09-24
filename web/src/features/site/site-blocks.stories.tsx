import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { ContactDetails, CtaBand, DocumentGrid, FeatureGrid, NewsList, ProductColumns, ReferenceGrid } from './site-blocks';

const meta = { title: 'Features/Site/Blocks', component: ReferenceGrid, parameters: { layout: 'padded' } } satisfies Meta<typeof ReferenceGrid>;
export default meta;
type Story = StoryObj<typeof meta>;

export const References: Story = {};
export const ReferenceLogosOnly: Story = { args: { stories: false } };
export const Documents: Story = { render: () => <DocumentGrid level={2} /> };
export const News: Story = { render: () => <NewsList /> };
export const Products: Story = { render: () => <ProductColumns /> };
export const Features: Story = { render: () => <FeatureGrid items={['Gerçek zamanlı izleme', 'Fatura hesaplama', 'Alarmlar']} /> };
export const Contact: Story = { render: () => <ContactDetails /> };
export const Cta: Story = { render: () => <CtaBand title="EkoKod'u kendi binanızla görün" href="/request-demo" action="Demo talep et" /> };
