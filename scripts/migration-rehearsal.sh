#!/usr/bin/env bash
# F14c R444: one timed rehearsal of the whole legacy migration into a SCRATCH
# database — extract → transform → artifacts → load → recompute → reconcile.
# The legacy database is only read. See docs/runbook-migration.md.
#
# Required: EKOKOD_* (the new stack's configuration, pointing at the scratch
# database), EKOKOD_LEGACY_ENCRYPTION_KEY, FROM (first legacy month, YYYY-MM),
# and either LEGACY_URI + LEGACY_DB or EXTRACT=<existing extract dir>.
# Optional: ARTIFACT_SOURCES (comma-separated legacy roots), ANSWERS,
# CONFIRMATIONS, WORKDIR (default ./rehearsal), EKOKOD (binary, default: build).
set -euo pipefail

: "${FROM:?FROM=YYYY-MM (the first legacy month) is required}"
: "${EKOKOD_DB_URL:?EKOKOD_DB_URL must point at the scratch database}"
if [[ "${EKOKOD_ENV:-}" == "production" ]]; then
  echo "refusing: a rehearsal never runs against production (EKOKOD_ENV=production)" >&2
  exit 1
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
work="${WORKDIR:-${root}/rehearsal}"
stamp="$(date +%Y%m%d-%H%M%S)"
mkdir -p "${work}" "${root}/reports"
log="${root}/reports/rehearsal-${stamp}.txt"
bin="${EKOKOD:-}"
if [[ -z "${bin}" ]]; then
  bin="${work}/ekokod"
  (cd "${root}" && go build -o "${bin}" ./cmd/ekokod)
fi

step() {
  local name="$1"; shift
  local start end
  start=$(date +%s)
  echo "== ${name}: $*" | tee -a "${log}"
  if "$@" >>"${log}" 2>&1; then
    end=$(date +%s)
    echo "   ${name}: ok in $((end - start)) s" | tee -a "${log}"
  else
    local rc=$?
    end=$(date +%s)
    echo "   ${name}: FAILED (exit ${rc}) after $((end - start)) s — see ${log}" | tee -a "${log}"
    return "${rc}"
  fi
}

extract="${EXTRACT:-${work}/extract}"
out="${work}/transform"
echo "rehearsal ${stamp} — database ${EKOKOD_DB_URL%%\?*}, from ${FROM}" | tee "${log}"

if [[ -z "${EXTRACT:-}" ]]; then
  : "${LEGACY_URI:?LEGACY_URI and LEGACY_DB (or EXTRACT) are required}"
  step inventory "${bin}" migrate legacy inventory --uri "${LEGACY_URI}" --db "${LEGACY_DB}" --json "${work}/inventory.json"
  step extract "${bin}" migrate legacy extract --uri "${LEGACY_URI}" --db "${LEGACY_DB}" --out "${extract}"
else
  step inventory "${bin}" migrate legacy inventory --from-extract "${extract}" --json "${work}/inventory.json"
fi

transform=("${bin}" migrate legacy transform --extract "${extract}" --out "${out}")
[[ -n "${ANSWERS:-}" ]] && transform+=(--answers "${ANSWERS}")
[[ -n "${CONFIRMATIONS:-}" ]] && transform+=(--confirmations "${CONFIRMATIONS}")
step transform "${transform[@]}"
if [[ -n "${ARTIFACT_SOURCES:-}" ]]; then
  step artifacts "${bin}" migrate legacy artifacts --dir "${out}" --source "${ARTIFACT_SOURCES}"
else
  echo '{"referenced":0,"copied":0,"already_there":0,"total_bytes":0,"by_owner_type":{},"referenced_missing":[],"path_escape":[],"unreferenced":[]}' >"${out}/artifacts_report.json"
  : >"${out}/stored_files.ndjson"
  echo "   artifacts: skipped (no ARTIFACT_SOURCES)" | tee -a "${log}"
fi
step schema "${bin}" migrate up
step reference-data "${bin}" seed
step load "${bin}" migrate legacy load --dir "${out}"
step recompute-consumption "${bin}" recompute consumption --from "${FROM}-01" --report "${work}/recompute-consumption.json"
# Bills, reports and carbon collect failures and still finish; the rehearsal goes on.
step recompute-bills "${bin}" recompute bills --from "${FROM}" --report "${work}/recompute-bills.json" || true
step recompute-reports "${bin}" recompute reports --from "${FROM}" --report "${work}/recompute-reports.json" || true
step recompute-carbon "${bin}" recompute carbon --from "${FROM}-01" --report "${work}/recompute-carbon.json" || true
step reconcile "${bin}" migrate legacy reconcile --dir "${out}" --extract "${extract}" --out "${root}/reports" || true
echo "rehearsal log: ${log}; report: ${root}/reports/reconciliation.html" | tee -a "${log}"
