package gridbox

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// MapStoredRow maps one GridBox energy value the legacy system stored verbatim,
// with the ingestion mapping: the provider's *WithMultiplier register when
// present, else raw × m (06 §3, F14a R409). field names what failed.
func MapStoredRow(raw json.RawMessage, analyzerID uuid.UUID, kind model.ReadingKind, m decimal.Decimal) (model.MeterReading, string, error) {
	var r register
	if err := json.Unmarshal(raw, &r); err != nil {
		return model.MeterReading{}, "row", err
	}
	ts, field, err := resolveTimestamp(r)
	if err != nil {
		if field == "" {
			field = "timestamp"
		}
		return model.MeterReading{}, field, err
	}
	reading, err := mapRegisters(r, m)
	if err != nil {
		return model.MeterReading{}, "registers", err
	}
	reading.AnalyzerID, reading.Ts, reading.Kind = analyzerID, ts, kind
	reading.MultiplierApplied = m
	reading.SourceProvider = model.IntegrationProviderGridbox
	reading.Raw = append(json.RawMessage(nil), raw...)
	return reading, "", nil
}
