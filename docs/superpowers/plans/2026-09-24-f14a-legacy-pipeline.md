# F14a — Legacy Migration Pipeline Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans`. Inline, one session, no subagents.

**Goal:** The read side of 08 §3–§5: `ekokod migrate legacy inventory | extract | transform`. It is idempotent, read-only against Mongo, and never drops anything silently. It covers the collections everything else hangs from:
- companies, together with their credentials, weekend days, vacations and calendar events;
- users;
- buildings, with contacts but no tariffs;
- analyzers;
- meter readings.

The remaining collections and `load` are F14b. `recompute`, `reconcile` and the runbook are F14c.

**Architecture:** `internal/migrate/legacy` holds these pieces:
- **Identity** (`ident.go`): UUIDv5 over (collection, ObjectId hex).
- **Normalisation** (`normalize.go`): dates, numbers and enums.
- **Rejections** (`rejects.go`): NDJSON with a reason, plus the count invariant input = accepted + rejected.
- **Legacy crypto** (`legacycrypto.go`): the legacy AES-256-CBC scheme (`enc:<iv>:<cipher>`, key = SHA-256(secret), optional fallback).
- **Source**: a read-only `Source` interface. The Mongo implementation uses find/count/listCollections only, behind a command monitor that fails any write.
- **Extract** (`extract.go`): deterministic gzip NDJSON plus a manifest with counts and SHA-256.
- **Inventory** (`inventory.go`).
- **Transform** (`transform*.go`): turns the extract into COPY-ready NDJSON per target table plus `legacy_ids.ndjson`.

The CLI lives in `internal/cli/migrate_legacy.go`.

**Spec:** 08 §1–§5, §11; 09 §F14; 06-integrations (multiplier rules); legacy models `bcem-energy/src/utils/db/models/*.ts`; legacy `src/utils/encryption.ts`; auth R145 (`legacy$` bcrypt hashes).

## Open questions (defaults shipped)

| # | Question | Default | Cost if wrong |
|---|---|---|---|
| Q-J1 | Command names. | **`ekokod migrate legacy <step>`** (`ekokod migrate` is the schema migrator; the product is no longer "bcem") and **`ekokod recompute <what>`** (F14c) | — |
| Q-J2 | The `legacy_id` column "on every migrated row". | **One `legacy_ids(collection, legacy_id, table_name, new_id)` table** (F14b migration) instead of 30 new columns. UUIDv5 makes the id itself derivable, so traceability is one join | A join instead of a column |
| Q-J3 | Credentials in staging files. | **Decrypted and immediately re-sealed** with the new key and the repository's AAD (company, definition). Plaintext never touches disk | — |
| Q-J4 | Legacy password hashes. | Stored as `legacy$<bcrypt>` (R145 verifies them and upgrades at login). Anything that is not bcrypt is rejected: that user gets a forced reset | — |
| Q-J5 | Multiplier undetermined. | The analyzer's readings are still transformed **as recorded**, with `multiplier_applied = null`. The analyzer is listed in `manual_multipliers.csv`, and `load` refuses them until confirmed (F14b) | — |
| Q-J6 | No legacy dump or Mongo available here. | The Mongo source is a thin adapter over the `Source` interface. Every behaviour is tested on an in-memory source with fixtures shaped like the legacy models. The Mongo integration test and the three rehearsals are PENDING, which needs a dump plus Docker | Mongo-specific quirks surface at the first rehearsal |

## Rulings (R400–R412)

| Id | Rule |
|---|---|
| R400 | **IDs:** `uuid.NewSHA1(ns, collection + ":" + hex)` with a fixed namespace; the same input always gives the same id. |
| R401 | **Dates:** accept ISO 8601 with an offset, `yyyy-MM-dd`, `dd-MM-yyyy`, `dd.MM.yyyy`, `dd/MM/yyyy`, `yyyy-MM` (month), `dd-MMM-yyyy` (Turkish or English), and `yyyy-MM-dd HH:mm[:ss]`. Naive timestamps are Europe/Istanbul → UTC. A two-digit year, or a value two layouts read differently (e.g. `03/04/2024` as d/m vs m/d), is **rejected** (`ambiguous_date`), never guessed. |
| R402 | **Numbers:** `1.234,56` and `1,234.56` both parse, **only** when the separators are unambiguous. `1.234` alone is ambiguous → rejected unless a field declares its locale. Empty → null. |
| R403 | **Enums:** subscriber groups (mesken→residential, ticarethane→commercial, sanayi→industrial, tarim/tarımsal→agricultural, aydinlatma→lighting), voltage (ag/og→lv/mv), term (tek/çift terim), tariff (tek/çok zamanlı). Case-folded in Turkish; anything unknown is rejected. |
| R404 | **Rejections:** every rejected record → `rejects/<collection>.ndjson` `{legacy_id, field, reason, value}`. A per-collection summary asserts `input = accepted + rejected`, and the transform fails if it does not hold. |
| R405 | **Extract:** one `<collection>.ndjson.gz` per collection, documents in `_id` order, canonical extended JSON, a gzip header without name or mtime. `manifest.json` {collection, count, sha256}. Re-running over unchanged data gives byte-identical files. |
| R406 | **Read-only:** the Mongo adapter issues only `find`, `count`/`countDocuments`/`aggregate` (read-only stages) and `listCollections`. A command monitor aborts on any other command (insert, update, delete, findAndModify, create, drop…). A unit test covers the classifier; the adapter's integration test is PENDING. |
| R407 | **Inventory:** the §3 list, printed as text and JSON: entity counts, users by role, analyzers by provider, per-analyzer reading counts by kind with first/last ts, gaps > 24 h, negative index deltas, missing multipliers; tariffs (count, date formats, missing VAT, PTF+YEKDEM without energy KBK); invoices by period with totals; carbon/ISO counts; artifact files per directory and orphans both ways. |
| R408 | **Meter readings:** `energyValues.{loadProfile,daily,billing,reset}` → one row per entry with `kind`. The same (analyzer, ts, kind) → **last wins**, and duplicates are counted. Future timestamps, or ones before 2010-01-01, are rejected. Negative deltas → a `consumption_anomalies` row (unresolved). ARIL `0` T1/T2/T3 → null. |
| R409 | **Multiplier (06 §multiplier resolution):**<br>• GridBox: take `*WithMultiplier` when present (applied = true); else raw × `Multiplier` field (applied = true); else undetermined.<br>• OSOS/Aril values: the provider returns multiplied values (applied = true).<br>• Others: undetermined → Q-J5. |
| R410 | **Credentials:** company `integrations[]` → `integration_credentials`. `enc:` secrets are decrypted with `--legacy-key` (and `--legacy-key-fallback`) and re-sealed (Q-J3). A value that fails to decrypt is rejected (`decrypt_failed`). Without `--legacy-key` the transform refuses to start. |
| R411 | **Users:** the role maps from `userType`; `passwordHistory` → `user_password_history` (same `legacy$` form); an unknown role is rejected. |
| R412 | **Idempotency:** transform output is a pure function of the extract plus the flags. `TestIdempotent` runs extract → transform twice and compares every output byte. |

## Tasks
1. Identity, normalisation (dates, numbers, enums), rejection writer + tests (R400–R404).
2. Legacy crypto + credential re-sealing (R410) + tests against vectors produced by the legacy algorithm.
3. Source interface, in-memory source, deterministic extract + manifest, the Mongo adapter with the write-guarding monitor (R405, R406).
4. Inventory (R407).
5. Transform: companies (+credentials, weekend, vacations, events), users, buildings (+contacts), analyzers, meter readings with multiplier/duplicates/anomalies (R408, R409, R411); TestIdempotent (R412).
6. CLI `ekokod migrate legacy inventory|extract|transform`, handoff.
