import { describe, expect, it } from 'vitest';

import { expectNoAxeViolations } from '@/test/axe';
import { renderWithProviders } from '@/test/render';

import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from './table';

function Example() {
  return (
    <TableContainer label="Binalar">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Bina</TableHead>
            <TableHead numeric>Tüketim (kWh)</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>Merkez</TableCell>
            <TableCell numeric>1.234,5</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </TableContainer>
  );
}

describe('Table', () => {
  it('scroll container is a focusable named region and numbers are tabular', () => {
    const { getByRole, getByText } = renderWithProviders(<Example />);
    expect(getByRole('region', { name: 'Binalar' })).toHaveAttribute('tabindex', '0');
    expect(getByText('1.234,5')).toHaveClass('text-end', 'type-data');
  });

  it('has no axe violations', async () => {
    const { container } = renderWithProviders(<Example />);
    await expectNoAxeViolations(container);
  });
});
