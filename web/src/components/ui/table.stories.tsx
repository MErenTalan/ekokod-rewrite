import type { Meta, StoryObj } from '@storybook/nextjs-vite';

import { Table, TableBody, TableCell, TableContainer, TableFooter, TableHead, TableHeader, TableRow } from './table';

const rows = [
  ['Aktif enerji', '182.345,120', '₺2,1840', '₺398.241,74'],
  ['Dağıtım bedeli', '182.345,120', '₺0,4120', '₺75.126,19'],
  ['Reaktif ceza', '4.210,000', '₺1,3100', '₺5.515,10'],
];

const meta = { title: 'UI/Table', component: Table } satisfies Meta<typeof Table>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <TableContainer label="Fatura kalemleri">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Kalem</TableHead>
            <TableHead numeric>Miktar (kWh)</TableHead>
            <TableHead numeric>Birim fiyat</TableHead>
            <TableHead numeric>Tutar</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(([a, ...rest]) => (
            <TableRow key={a}>
              <TableCell>{a}</TableCell>
              {rest.map((v, i) => (
                <TableCell key={i} numeric>
                  {v}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
        <TableFooter>
          <TableRow>
            <TableCell colSpan={3}>Toplam</TableCell>
            <TableCell numeric>₺478.883,03</TableCell>
          </TableRow>
        </TableFooter>
      </Table>
    </TableContainer>
  ),
};
