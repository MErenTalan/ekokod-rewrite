# İcmal test fixture

`ck_icmal_2025_11_12_anonymised.csv` is a real supplier icmal (CK Enerji, accounting periods 202511 and
202512, 45 data rows) from the legacy repository root (`icmalverileri.csv`), anonymised for commit.

Anonymisation (Python `csv`, minimal quoting, LF, verified byte-identical on an unmodified round trip):

- Dropped columns: İl, İlçe, Ad Soyad (both), VKN (both), TCKN (both), Hesap No, Sayac Seri Numarası,
  Sayaç No, Dağıtım Hizmet Noktası No, Pmum Sayaç Id, Fatura Seri No, Fatura No.
- Each distinct ETSO code replaced by `40ZTEST` + a 9-digit sequence in first-seen order (37 codes).
- Every other column and value is unchanged. Checked: no run of 10+ digits, no original `40Z0` prefix.

Hand sums over all 45 rows (for `TestParseRealFixtureRowCountAndTotals`):

- Σ Toplam Kwh = 1175379.55
- Σ Fatura Tutarı = 16670670.00

Synthetic base prices used by the derivation tests (not real PTF/YEKDEM): 202511 → 2.5 TL/kWh,
202512 → 2.6 TL/kWh.

Row map used by tests (CSV line → sequence): line 10 → 40ZTEST000000009 (binomial, power 17348.01);
line 31 → 40ZTEST000000030 (power 13011.01 + overrun 12282.39, demand 441.60); lines 2/34/36 →
40ZTEST000000001, lines 39/43/45 → 40ZTEST000000036, lines 40/44/46 → 40ZTEST000000037 (each group:
one zero-kWh row, one regular row, one `(Ek)` supplementary correction); line 20 → 40ZTEST000000019
(reactive charge with both registers 0).

The supplier's `Fatura Tutarı` is an account-level rounded payable, never an oracle (I-2).
