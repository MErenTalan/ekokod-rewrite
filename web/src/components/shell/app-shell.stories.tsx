import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { formatNumber } from '@/lib/format';

import { DateRangePicker } from '../ui/date-range-picker';
import { Select } from '../ui/select';
import { StatTile } from '../ui/stat-tile';
import { storyUser } from './_story-user';
import { AppShell } from './app-shell';
import { FilterBar } from './filter-bar';
import { PageHeader } from './page-header';

const Page = () => (
  <>
    <PageHeader title="Yük Profili" description="Merkez Bina · son 30 gün" />
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <StatTile label="Toplam tüketim" value={formatNumber('182345.1')} unit="kWh" />
      <StatTile label="Tepe güç" value={formatNumber('612.4')} unit="kW" />
      <StatTile label="Ortalama güç" value={formatNumber('253.3')} unit="kW" />
      <StatTile label="Yük faktörü" value={formatNumber('41.4')} unit="percent" />
    </div>
  </>
);

const meta = {
  title: 'Shell/AppShell',
  component: AppShell,
  args: { user: storyUser, notificationCount: 3, children: <Page /> },
  parameters: { layout: 'fullscreen', nextjs: { appDirectory: true, navigation: { pathname: '/ekorm/load-profile' } } },
} satisfies Meta<typeof AppShell>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const Horizontal: Story = { parameters: { preferences: { layout: 'horizontal' } } };
export const Collapsed: Story = { parameters: { preferences: { sidebar: 'collapsed' } } };
export const Boxed: Story = { parameters: { preferences: { container: 'boxed', card: 'shadow' } } };
export const WithFilterBar: Story = {
  args: {
    children: (
      <>
        <PageHeader title="Tüketim" description="Bina ve dönem seçerek saatlik tüketimi inceleyin" />
        <FilterBar activeCount={2} onApply={() => {}}>
          <div className="w-56">
            <Select label="Çözünürlük" options={[{ value: 'hourly', label: 'Saatlik' }, { value: 'daily', label: 'Günlük' }]} value="daily" onValueChange={() => {}} />
          </div>
          <div className="w-72">
            <DateRangePicker label="Dönem" value={{ from: '2026-08-01', to: '2026-08-31' }} onValueChange={() => {}} />
          </div>
        </FilterBar>
        <Page />
      </>
    ),
  },
  parameters: { nextjs: { appDirectory: true, navigation: { pathname: '/ekorm/consumption' } } },
};
