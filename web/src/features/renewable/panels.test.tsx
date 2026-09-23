import { describe, expect, it } from 'vitest';

import { renderWithProviders } from '@/test/render';

import {
  analytics,
  efficiency,
  environmental,
  forecast,
  grid,
  realtime,
  systemStatus,
} from './_fixture';
import {
  AnalyticsPanelView,
  EfficiencyPanelView,
  EnvironmentalPanelView,
  ForecastPanelView,
  GridPanelView,
  RealtimePanelView,
  SystemStatusPanelView,
} from './panels';

describe('renewable panels (R292)', () => {
  it('realtime: last hour, status, and an efficiency it cannot measure', () => {
    const r = renderWithProviders(<RealtimePanelView data={realtime} />);
    expect(r.getByRole('heading', { name: 'Gerçek zamanlı üretim' })).toBeVisible();
    expect(r.getByText('42,5 kW')).toBeVisible();
    expect(r.getByText('Üretiyor')).toBeVisible();
    expect(r.getByText('Işınım ya da kurulu güç ölçülmüyor; verim hesaplanamaz.')).toBeVisible();
  });

  it('grid: voltage and frequency say why they are missing, never 0', () => {
    const r = renderWithProviders(<GridPanelView data={grid} />);
    expect(r.getByText('Şebekeye veriş')).toBeVisible();
    expect(r.getAllByText('Gerilim ve frekans ölçülmüyor.')).toHaveLength(2);
    expect(r.queryByText('0 V')).toBeNull();
    expect(r.getByText('0,97')).toBeVisible();
  });

  it('environmental: CO₂ at the grid factor, each equivalence with its source (R293)', () => {
    const r = renderWithProviders(<EnvironmentalPanelView data={environmental} />);
    expect(r.getByText('550,22 kg')).toBeVisible();
    expect(r.getByText('Kayıtlı katsayı yok.')).toBeVisible();
    expect(r.getByText(/Kaynak: EEA \(2023\)/)).toBeVisible();
    expect(r.getByText(/Kaynak: TEİAŞ/)).toBeVisible();
    expect(r.getByText(/1 ağaç-yıl = 21,77 kg CO2\/ağaç-yıl/)).toBeVisible();
  });

  it('efficiency: every figure is unavailable with its reason', () => {
    const r = renderWithProviders(<EfficiencyPanelView data={efficiency} />);
    expect(r.getAllByText('veri yok').length).toBeGreaterThanOrEqual(5);
    expect(r.getByText('Batarya ölçümü yok.')).toBeVisible();
    expect(r.queryByText(/%0/)).toBeNull();
  });

  it('forecast: sums from stored runs; accuracy without overlap is explained', () => {
    const r = renderWithProviders(<ForecastPanelView data={forecast} />);
    expect(r.getByText('310,5 kWh')).toBeVisible();
    expect(r.getByText('%91,2')).toBeVisible();
    expect(r.getByText('Tahminlerle ölçümler çakışmıyor; doğruluk hesaplanamaz.')).toBeVisible();
    expect(r.getAllByText('Üretim tahmini yapılmıyor.')).toHaveLength(3);
  });

  it('analytics: peak hour, trend and the financial gains from bills', () => {
    const r = renderWithProviders(<AnalyticsPanelView data={analytics} />);
    expect(r.getByText('12:00')).toBeVisible();
    expect(r.getByRole('heading', { name: 'Finansal kazanç' })).toBeVisible();
    expect(r.getByText(/9\.800,75/)).toBeVisible();
    expect(r.getAllByText('Yatırım maliyeti kayıtlı değil.')).toHaveLength(2);
  });

  it('system status: component states and the ones without telemetry', () => {
    const r = renderWithProviders(<SystemStatusPanelView data={systemStatus} />);
    expect(r.getAllByText('Sağlıklı')).toHaveLength(3);
    expect(r.getAllByText('Bu bileşenden telemetri gelmiyor.')).toHaveLength(3);
  });

  it('a panel whose read failed says so instead of loading forever', () => {
    const r = renderWithProviders(<GridPanelView />);
    expect(r.getByText('Bu panel yüklenemedi.')).toBeVisible();
  });

  it('a panel still loading shows a skeleton, not "veri yok"', () => {
    const r = renderWithProviders(<RealtimePanelView loading />);
    expect(r.queryByText('veri yok')).toBeNull();
    expect(r.getByRole('heading', { name: 'Gerçek zamanlı üretim' })).toBeVisible();
  });
});
