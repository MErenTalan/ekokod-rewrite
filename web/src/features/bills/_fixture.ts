import type { Analyzer, BillDashboard } from '@/lib/api/types';

/** Story and test data for the bills screen; never fetched (R196). */
export const demoDashboard: BillDashboard = {
  period: '2026-08',
  plants: {
    available: true,
    rows: [
      {
        plant_id: 'p-1', plant_name: 'Konya GES', analyzer_name: 'Mahsup Sayacı', installation_number: '40009',
        production_kwh: '1200', production_price: '1.8', currency: 'TRY', consumption_price: '3.12',
        invoice_amount: '2160.00', bill_id: 'bill-1',
      },
      {
        plant_id: 'p-2', plant_name: 'Arazi GES', analyzer_name: undefined, installation_number: undefined,
        production_kwh: undefined, production_price: undefined, currency: undefined, consumption_price: undefined,
        invoice_amount: undefined, bill_id: undefined,
      },
    ],
    total_production_kwh: '1200',
    total_invoice: [{ currency: 'TRY', amount: '2160.00' }],
  },
  buildings: [
    {
      building_id: 'b-1',
      building_name: 'A1 Fabrika',
      currency: 'TRY',
      diverges_from_rows: false,
      total_consumption: '201',
      total_production: '0',
      total_invoice: '510',
      building_bill: undefined,
      rows: [
        {
          bill_id: 'bill-1', building_id: 'b-1', analyzer_id: 'a-1', building_name: 'A1 Fabrika',
          analyzer_name: 'Sayaç 1', installation_number: '40001', etso_code: '40Z0000000001A',
          period_key: '2026-08', consumption: '120.5', production: '0', consumption_price: '3.12',
          production_price: undefined, invoice: '310.25', currency: 'TRY',
        },
        {
          bill_id: 'bill-2', building_id: 'b-1', analyzer_id: 'a-2', building_name: 'A1 Fabrika',
          analyzer_name: 'Sayaç 2', installation_number: '40002', etso_code: '40Z0000000002B',
          period_key: '2026-08', consumption: '80.5', production: '0', consumption_price: '3.12',
          production_price: undefined, invoice: '199.75', currency: 'TRY',
        },
      ],
    },
  ],
  netting: [
    {
      currency: 'TRY', total_consumption: '201', total_production: '0', net: '201',
      net_status: 'net_consumption', total_invoice: '510', efficiency_pct: '0',
      period_key: '2026-08', company_bill_id: undefined,
    },
  ],
};

/** The same month with a building invoice that 02 §6.11 priced differently. */
export const demoDivergentDashboard: BillDashboard = {
  ...demoDashboard,
  buildings: [
    {
      ...demoDashboard.buildings[0],
      diverges_from_rows: true,
      building_bill: {
        bill_id: 'bill-b', building_id: 'b-1', analyzer_id: undefined, building_name: 'A1 Fabrika',
        analyzer_name: '', installation_number: '', etso_code: '', period_key: '2026-08',
        consumption: '201', production: '0', consumption_price: '3.1', production_price: undefined,
        invoice: '495', currency: 'TRY',
      },
    },
  ],
};

export const demoAnalyzers: Analyzer[] = [
  {
    id: 'a-1', company_id: 'c-own', building_id: 'b-1', provider: 'aril', provider_subtype: 'aril',
    installation_number: '40001', customer_name: 'A1 Fabrika A.Ş.', province: 'Ankara', district: 'Çankaya',
    meter_multiplier: '1', is_active: true, activity_status: 'active', created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  } as Analyzer,
  {
    id: 'a-2', company_id: 'c-own', building_id: 'b-1', provider: 'aril', provider_subtype: 'aril',
    installation_number: '40002', customer_name: 'A1 Fabrika A.Ş.', province: 'Ankara', district: 'Çankaya',
    meter_multiplier: '1', is_active: true, activity_status: 'active', created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  } as Analyzer,
];
