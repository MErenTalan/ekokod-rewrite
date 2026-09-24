import type { FinancialMonth, FinancialMonthly, FinancialSummary } from '@/lib/api/types';

const empty = (month: number): FinancialMonth => ({
  month,
  cost: [],
  revenue: [],
  net: [],
  revenue_partial: false,
});

const filled: Record<number, FinancialMonth> = {
  1: {
    month: 1,
    consumption_kwh: '1500',
    cost: [{ currency: 'TRY', amount: '4600.00' }],
    production_kwh: '1200',
    revenue: [{ currency: 'TRY', amount: '2400.00' }],
    revenue_partial: false,
    offset_kwh: '-300',
    grid_purchase_kwh: '300',
    grid_sale_kwh: '0',
    net: [{ currency: 'TRY', amount: '2200.00' }],
  },
  2: {
    month: 2,
    consumption_kwh: '1200',
    cost: [
      { currency: 'TRY', amount: '3750.00' },
      { currency: 'EUR', amount: '120.00' },
    ],
    production_kwh: '300',
    revenue: [{ currency: 'TRY', amount: '750.00' }],
    revenue_partial: true,
    offset_kwh: '-900',
    grid_purchase_kwh: '900',
    grid_sale_kwh: '0',
    net: [
      { currency: 'TRY', amount: '3000.00' },
      { currency: 'EUR', amount: '120.00' },
    ],
  },
  3: {
    month: 3,
    consumption_kwh: '900',
    cost: [{ currency: 'TRY', amount: '2800.00' }],
    revenue: [],
    revenue_partial: false,
    net: [{ currency: 'TRY', amount: '2800.00' }],
  },
};

export const monthly: FinancialMonthly = {
  items: Array.from({ length: 12 }, (_, i) => filled[i + 1] ?? empty(i + 1)),
  total: {
    month: 0,
    consumption_kwh: '3600',
    cost: [
      { currency: 'TRY', amount: '11150.00' },
      { currency: 'EUR', amount: '120.00' },
    ],
    production_kwh: '1500',
    revenue: [{ currency: 'TRY', amount: '3150.00' }],
    revenue_partial: true,
    offset_kwh: '-2100',
    grid_purchase_kwh: '2100',
    grid_sale_kwh: '0',
    net: [
      { currency: 'TRY', amount: '8000.00' },
      { currency: 'EUR', amount: '120.00' },
    ],
  },
  coverage: { consumption_months: 3, production_months: 2, of: 12 },
};

export const summary: FinancialSummary = {
  year: 2026,
  analyzer_count: 14,
  plant_count: 3,
  figures: monthly.total,
  with_data: 9,
  of: 12,
  tariffs: {
    purchase: [
      {
        building_id: 'b-1',
        building_name: 'A1 Fabrika',
        price_type: 'multi_time',
        t1: '2.95',
        t2: '4.4',
        t3: '1.8',
        currency: 'TRY',
      },
      {
        building_id: 'b-2',
        building_name: 'A2 Depo',
        price_type: 'single_time',
        single: '3.1',
        currency: 'TRY',
      },
    ],
    sale: [{ plant_id: 'p-1', plant_name: 'Konya GES', price: '2.5', currency: 'TRY' }],
    purchase_missing: false,
    sale_missing: false,
  },
};
