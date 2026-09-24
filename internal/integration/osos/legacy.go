package osos

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// MapStoredRow maps one energy value the legacy system stored in OSOS's own
// shape, with the ingestion mapping itself, so the migration and live ingestion
// can never disagree on a register (F14a R409). m is applied exactly once.
func MapStoredRow(raw json.RawMessage, analyzerID uuid.UUID, kind model.ReadingKind, m decimal.Decimal) (model.MeterReading, string, bool) {
	return mapEnergyRow(raw, integration.FetchRequest{AnalyzerID: analyzerID, Kind: kind, Multiplier: m})
}
