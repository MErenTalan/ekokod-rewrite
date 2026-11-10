# F31 — SKTT Tebliğ pricing (R565–R567), 2026-11-10

**Problem.** Fuga Mobilya's October bill (MEPAŞ, reads 16.09 → 17.10.2026) came out 40,614 TL at
MEPAŞ and 39,341.57 TL here, flagged "PTF verisi eksik".

**Root causes.**
1. PTF was complete (744/744 October hours, equal to live EPİAŞ). The flag was October **YEKDEM**
   missing — EPİAŞ publishes the actual a month or more later — and the label called it PTF.
2. The energy price method. MEPAŞ applies the SKTT Tebliğ (RG 30307 md. 6), not the hourly
   consumption-weighted PTF: daily volume-weighted PTF averaged over the read dates with the two end
   dates at half weight, plus the EPDK forecast YEKDEM. That reproduces all three MEPAŞ bills
   (3.593417 / 3.774401 / 3.616873 TL/kWh) to +0.000032 TL/kWh; the hourly method was −3.9 %.

**Changes.**
- R565: `market_prices_hourly.matched_quantity` (migration 00029), fetched from EPİAŞ
  `/v1/markets/dam/data/clearing-quantity` by the price sync.
- R566: `yekdem_forecast` table, seeded with EPDK decision 14718 (Jul–Dec 2026).
- R567: incumbent PTF tariffs priced by the Tebliğ (02 §7.2a); new flag `yekdem_data_missing`.

**After deploy.** `ekokod recompute prices --from 2026-06-01` backfills the matched quantities;
add EPDK's 2027 forecast rows to `yekdem_forecast` when the decision is published (December).
