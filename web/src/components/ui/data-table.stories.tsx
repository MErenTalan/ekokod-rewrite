import type { Meta, StoryObj } from '@storybook/nextjs-vite';
import type { ColumnDef } from '@tanstack/react-table';
import { Pencil, Trash2 } from 'lucide-react';

import { formatNumber } from '@/lib/format';

import { DataTable } from './data-table';
import { StatusBadge } from './status-badge';

type Building = { id: string; name: string; city: string; active: boolean; kwh: string; cost: string; inductive: string };
const cities = ['İstanbul', 'Ankara', 'İzmir', 'Bursa', 'Kocaeli', 'Gaziantep', 'Şanlıurfa', 'Çorum'];
const kinds = ['Merkez Bina', 'Soğuk Hava Deposu', 'Üretim Tesisi', 'İdari Ofis', 'GES Sahası'];
const data: Building[] = Array.from({ length: 40 }, (_, i) => ({
  id: `b${i}`,
  name: `${cities[i % cities.length]} ${kinds[i % kinds.length]}`,
  city: cities[i % cities.length],
  active: i % 7 !== 3,
  kwh: `${(i * 7919) % 250000}.${String((i * 37) % 1000).padStart(3, '0')}`,
  cost: `${(i * 104729) % 900000}.${String((i * 13) % 100).padStart(2, '0')}`,
  inductive: `${(i * 3) % 30}.${i % 10}`,
}));

const byNumber = (key: keyof Building) => (a: { original: Building }, b: { original: Building }) => Number(a.original[key]) - Number(b.original[key]);
const columns: ColumnDef<Building, unknown>[] = [
  { accessorKey: 'name', header: 'Bina', enableHiding: false },
  { accessorKey: 'city', header: 'Şehir' },
  { accessorKey: 'active', header: 'Durum', cell: ({ row }) => <StatusBadge status={row.original.active ? 'success' : 'neutral'} label={row.original.active ? 'Aktif' : 'Pasif'} /> },
  { accessorKey: 'kwh', header: 'Tüketim (kWh)', meta: { numeric: true, exportValue: (r) => formatNumber(r.kwh) }, cell: ({ row }) => formatNumber(row.original.kwh), sortingFn: byNumber('kwh') },
  { accessorKey: 'cost', header: 'Maliyet (₺)', meta: { numeric: true, exportValue: (r) => formatNumber(r.cost) }, cell: ({ row }) => formatNumber(row.original.cost, { minFractionDigits: 2 }), sortingFn: byNumber('cost') },
  { accessorKey: 'inductive', header: 'Endüktif oran (%)', meta: { numeric: true }, cell: ({ row }) => formatNumber(row.original.inductive), sortingFn: byNumber('inductive') },
];
const extra: ColumnDef<Building, unknown>[] = ['Sözleşme gücü (kW)', 'Trafo', 'Abone grubu', 'Tarife', 'Dağıtım şirketi', 'Sayaç no'].map((header, i) => ({
  id: `extra${i}`,
  header,
  accessorFn: (r: Building) => `${r.id.toUpperCase()}-${i}`,
}));
const empty = { title: 'Bina bulunamadı', description: 'Filtreleri temizleyin veya Ayarlar sayfasından bina ekleyin.' };

const meta = {
  title: 'UI/DataTable',
  component: DataTable<Building>,
  args: { columns, data, caption: 'Binalar', getRowId: (r: Building) => r.id, empty, enableColumnVisibility: true },
} satisfies Meta<typeof DataTable<Building>>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    getRowLabel: (r: Building) => r.name,
    rowActions: () => [
      { type: 'item', label: 'Düzenle', icon: Pencil, onSelect: () => {} },
      { type: 'separator' },
      { type: 'item', label: 'Sil', icon: Trash2, tone: 'danger', onSelect: () => {} },
    ],
    initialSorting: [{ id: 'kwh', desc: true }],
    maxHeight: '480px',
  },
};
export const Loading: Story = { args: { loading: true, data: [] } };
export const Empty: Story = { args: { data: [] } };
export const ManyColumns: Story = { args: { columns: [...columns, ...extra], pageSize: 10 } };
