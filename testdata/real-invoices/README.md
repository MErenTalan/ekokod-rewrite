# Real-invoice fixtures

`ekokod tool compare-invoice --fixture <file>.json` recomputes a fixture's `input` with the billing engine and prints
`code | expected | computed | diff | explanation`. It exits non-zero when any line differs without an explanation.

- `icmal_row_ck_anonymised.json` — CSV line 31 of the anonymised CK Enerji icmal
  (`internal/domain/tariff/icmal/testdata`). Unit prices are the supplier's amount / quantity, so this proves invoice
  *assembly* (BTV on energy, KDV matrahı, KDV, rounding) to the kuruş, not the supplier's prices.
- `expected.total` is KDV Matrahı + KDV. The supplier's `Fatura Tutarı` is an account-level rounded payable shared across
  rows (≈0.79 % below matrah + KDV on single-row accounts) and is never an oracle (I-2). **Open PO question.**

**§F4 acceptance item still OPEN:** the product owner supplies real invoices across free-market, regulated/Enerjisa,
(PTF+YEKDEM)×KBK and custom-power-charge tariffs; each becomes a fixture here with every difference explained.
