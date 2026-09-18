package alarm

import (
	"fmt"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// defaultLocale is the product's own (05 §1): tr, with en supported.
const defaultLocale = "tr"

// phrases is the breach sentence per field per locale. Every verb takes
// (measured, threshold) in that order, so a wrong pairing cannot pass
// unnoticed — the tests assert on both numbers.
//
// %% is a literal percent sign; "%%%s" therefore renders as "%25".
var phrases = map[string]map[string]string{
	"tr": {
		FieldInductive:  "Endüktif oran %%%s eşiği aştı (%%%s)",
		FieldCapacitive: "Kapasitif oran %%%s eşiği aştı (%%%s)",
		FieldActiveMax:  "Aktif tüketim %s kWh maksimum limiti aştı (%s kWh)",
		FieldActiveMin:  "Aktif tüketim %s kWh minimum limitin altına düştü (%s kWh)",
		FieldCommsHours: "Analizör %s saattir veri göndermiyor (eşik: %s saat)",
		FieldPowerMax:   "Güç %s kW maksimum limiti aştı (%s kW)",
		FieldPowerMin:   "Güç %s kW minimum limitin altına düştü (%s kW)",
		FieldInvoicePct: "Fatura %%%s arttı (eşik: %%%s)",
	},
	"en": {
		FieldInductive:  "Inductive ratio %s%% exceeded the threshold (%s%%)",
		FieldCapacitive: "Capacitive ratio %s%% exceeded the threshold (%s%%)",
		FieldActiveMax:  "Active consumption %s kWh exceeded the maximum (%s kWh)",
		FieldActiveMin:  "Active consumption %s kWh fell below the minimum (%s kWh)",
		FieldCommsHours: "The analyzer has sent no data for %s hours (threshold: %s hours)",
		FieldPowerMax:   "Power %s kW exceeded the maximum (%s kW)",
		FieldPowerMin:   "Power %s kW fell below the minimum (%s kW)",
		FieldInvoicePct: "The invoice rose by %s%% (threshold: %s%%)",
	},
}

// DescribableFields is every field a breach can name, so a guard can check
// that no locale is missing a sentence — a missing one would render as
// %!s(MISSING) inside an e-mail.
func DescribableFields() []string {
	return []string{FieldInductive, FieldCapacitive, FieldActiveMax, FieldActiveMin,
		FieldCommsHours, FieldPowerMax, FieldPowerMin, FieldInvoicePct}
}

// PhraseFor exposes one sentence template, for that guard.
func PhraseFor(locale, field string) string { return phrases[normaliseLocale(locale)][field] }

func normaliseLocale(locale string) string {
	if _, ok := phrases[locale]; !ok {
		return defaultLocale
	}
	return locale
}

// Describe renders the operator-facing sentences for a verdict and the curated
// detail object that lands in alarm_events.detail (R224).
//
// detail carries only four keys, by construction: nothing a provider or an
// SMTP server said can ride along into a row that operators read and export.
// A no-verdict is reported separately from the breaches, because "we could not
// decide" is neither "nothing is wrong" nor "something is wrong".
func Describe(a model.Alarm, v Verdict, analyzerLabel, locale string) (string, []string, map[string]any) {
	locale = normaliseLocale(locale)
	lines := make([]string, 0, len(v.Breaches))
	rows := make([]map[string]any, 0, len(v.Breaches))
	for _, b := range v.Breaches {
		lines = append(lines, fmt.Sprintf(phrases[locale][b.Field], b.Measured.String(), b.Threshold.String()))
		row := map[string]any{"field": b.Field, "measured": b.Measured.String(), "threshold": b.Threshold.String()}
		if !b.Window.From.IsZero() {
			row["window_from"] = b.Window.From.Format(time.RFC3339)
			row["window_to"] = b.Window.To.Format(time.RFC3339)
		}
		for k, val := range b.Extra {
			row[k] = val
		}
		rows = append(rows, row)
	}
	// The summary is the rule and the meter; the locale lives in the lines.
	summary := fmt.Sprintf("%s — %s", a.Name, analyzerLabel)
	// lines ride along in the detail so a later notifier says what the event
	// said, rather than re-deriving sentences from thresholds whose readings it
	// no longer has. It stays curated: five keys, all built here.
	detail := map[string]any{
		"analyzer": analyzerLabel, "alarm_type": string(a.Type), "breaches": rows, "lines": lines,
	}
	if len(v.NoVerdict) > 0 {
		detail["no_verdict"] = v.NoVerdict
	}
	return summary, lines, detail
}
