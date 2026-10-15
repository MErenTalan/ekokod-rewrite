# F26 — hourly netting for prosumers (R531)

**Status:** done 2026-10-15 on `fix/f26-hourly-netting`; merged and deployed the same day.

**Why:** the owner reported zero bills for Gaye Hanım EV (Aug: import 224, export 769 → net 0, everything 0).
The real Enerjisa bills (project root `Ağustos/Eylül 2026 faturası.html`) charge the whole import at one
all-in price (~1.2124 TL/kWh) and show generation only as "Üretim Miktarı". The PO (2026-10-15): generation
is netted hourly ("saatlik mahsuplaşma"), distribution is charged on consumption whatever the generation,
and residential producers sell the excess at a (cheap) sale price that should be recorded.

| Id | Ruling | Cost if wrong |
|----|--------|---------------|
| R531 | `subtract_from_consumption` nets **hourly**: overlap = Σ min(import_h, export_h) over sound hourly rows (`consumption.HourlyOverlap`; absorbed, partial and suspect hours skipped). Net = import − overlap; excess = export − overlap. Energy, green energy and per-kWh extras on the net; **distribution on the whole import** (every generation usage); R122's tier offset is dropped (tier limit = days × threshold). No hourly data → the import stays billed and `generation_data_missing` is flagged; never a monthly netting again. The excess sale is an informational `generation_sale` line (qty excess, unit price = tariff `generation_price_per_kwh` if set), appended after the total, outside VAT base and total, printed under the amount due on the PDF. The reactive screen divides by the same hourly net (R511). The tariff form shows an optional "Üretim satış fiyatı" for this usage. | A prosumer bill of 0 TL, or distribution and tiers wrong for every prosumer |

**After deploy:** fix Gaye Hanım's tariff (single all-in price ~1.2124, not energy 0.494 + distribution
1.2124; sale price when known) and force-regenerate Jul–Sep bills.
