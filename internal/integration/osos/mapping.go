package osos

// mapping.go holds only pure functions from wire structs to
// model.MeterReading / integration.MeteringPoint / integration.HourlyValue.
// Nothing here makes a network call or touches a clock beyond what the
// caller passes in.

import (
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// stringOrNil turns an empty provider string into a nil pointer
// (removed-behaviour 21's "unavailable is nil, never a zero value" applies
// to strings the same way it applies to numbers: OSOS's JSON always sends a
// string field, but an empty one means "the provider reported nothing").
func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optionalProviderNumber parses raw through normalize.ProviderNumber (R10),
// treating an empty string as "the provider reported nothing" (nil, nil)
// rather than a parse error — discovery rows leave several numeric fields
// blank for installations OSOS has no value for yet.
func optionalProviderNumber(raw string) (*decimal.Decimal, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	v, err := normalize.ProviderNumber(raw)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// mapInstallation converts one discovery-list row into a MeteringPoint,
// mapping every field of 06 §2's table. kuruluGucu, koordinatX/Y and
// meterMultiplier go through normalize.ProviderNumber (R10); everything
// else is a direct or nil-preserving string copy.
func mapInstallation(row wireInstallation) (integration.MeteringPoint, error) {
	installedPower, err := optionalProviderNumber(row.KuruluGucu)
	if err != nil {
		return integration.MeteringPoint{}, err
	}
	lat, err := optionalProviderNumber(row.KoordinatX)
	if err != nil {
		return integration.MeteringPoint{}, err
	}
	lon, err := optionalProviderNumber(row.KoordinatY)
	if err != nil {
		return integration.MeteringPoint{}, err
	}
	multiplier, err := optionalProviderNumber(row.MeterMultiplier)
	if err != nil {
		return integration.MeteringPoint{}, err
	}

	return integration.MeteringPoint{
		InstallationNumber: row.InstalationNumber,
		CustomerName:       stringOrNil(row.CustomerName),
		Address:            stringOrNil(row.CustomerAdress),
		Province:           stringOrNil(row.Il),
		District:           stringOrNil(row.Ilce),
		Neighbourhood:      stringOrNil(row.KoyMahallesi),
		Street:             stringOrNil(row.CaddesiSokagi),
		TariffType:         stringOrNil(row.TarifeTipi),
		TariffKind:         stringOrNil(row.TarifeTuru),
		InstallationKind:   stringOrNil(row.TesisatTurTanim),
		InstalledPowerKw:   installedPower,
		MeterNumber:        stringOrNil(row.MeterNumber),
		MeterModel:         stringOrNil(row.MeterModel),
		MeterMultiplier:    multiplier,
		CounterpartyNo:     stringOrNil(row.MuhatapNo),
		MeteringPointName:  stringOrNil(row.SayimNokTanim),
		Latitude:           lat,
		Longitude:          lon,
	}, nil
}

// mapEnergyRow converts one energy_values row into a model.MeterReading.
// ok is false when the row failed to parse (the "field" return names which
// one, for the caller's Warning.Detail); a failed row is never returned
// half-built. req.Multiplier is applied exactly once, here, via
// normalize.Multiply (R8) — never anywhere downstream.
func mapEnergyRow(row wireEnergyRow, req integration.FetchRequest) (reading model.MeterReading, field string, ok bool) {
	ts, err := normalize.OSOSDate(row.MeterDate)
	if err != nil {
		return model.MeterReading{}, "meter_date", false
	}

	type regField struct {
		name string
		raw  string
		dst  **decimal.Decimal
	}
	var (
		activeImport, reactiveIndImport, reactiveCapImport *decimal.Decimal
		t1Import, t2Import, t3Import, maxDemand            *decimal.Decimal
		activeExport, reactiveIndExport, reactiveCapExport *decimal.Decimal
		t1Export, t2Export, t3Export                       *decimal.Decimal
	)
	fields := []regField{
		{"t_top_kWh", row.TTop, &activeImport},
		{"t_ri_kVarh", row.TRi, &reactiveIndImport},
		{"t_rc_kVarh", row.TRc, &reactiveCapImport},
		{"t_t1_kWh", row.TT1, &t1Import},
		{"t_t2_kWh", row.TT2, &t2Import},
		{"t_t3_kWh", row.TT3, &t3Import},
		{"t_p_kW", row.TP, &maxDemand},
		{"u_top_kWh", row.UTop, &activeExport},
		{"u_ri_kVarh", row.URi, &reactiveIndExport},
		{"u_rc_kVarh", row.URc, &reactiveCapExport},
		{"u_u1_kWh", row.UU1, &t1Export},
		{"u_u2_kWh", row.UU2, &t2Export},
		{"u_u3_kWh", row.UU3, &t3Export},
		// u_p_kW (row.UP) is deliberately absent from this list — see
		// wire.go's doc comment on wireEnergyRow.
	}
	for _, f := range fields {
		v, err := normalize.OptionalNumber(f.raw)
		if err != nil {
			return model.MeterReading{}, f.name, false
		}
		*f.dst = v
	}

	raw, _ := json.Marshal(row)

	reading = model.MeterReading{
		AnalyzerID:               req.AnalyzerID,
		Ts:                       ts,
		Kind:                     req.Kind,
		ActiveImport:             normalize.Multiply(activeImport, req.Multiplier),
		ReactiveInductiveImport:  normalize.Multiply(reactiveIndImport, req.Multiplier),
		ReactiveCapacitiveImport: normalize.Multiply(reactiveCapImport, req.Multiplier),
		T1Import:                 normalize.Multiply(t1Import, req.Multiplier),
		T2Import:                 normalize.Multiply(t2Import, req.Multiplier),
		T3Import:                 normalize.Multiply(t3Import, req.Multiplier),
		ActiveExport:             normalize.Multiply(activeExport, req.Multiplier),
		ReactiveInductiveExport:  normalize.Multiply(reactiveIndExport, req.Multiplier),
		ReactiveCapacitiveExport: normalize.Multiply(reactiveCapExport, req.Multiplier),
		T1Export:                 normalize.Multiply(t1Export, req.Multiplier),
		T2Export:                 normalize.Multiply(t2Export, req.Multiplier),
		T3Export:                 normalize.Multiply(t3Export, req.Multiplier),
		MaxDemandKw:              normalize.Multiply(maxDemand, req.Multiplier),
		MeterSerial:              stringOrNil(row.MeterSerialNo),
		MultiplierApplied:        req.Multiplier,
		SourceProvider:           model.IntegrationProviderOSOS,
		Raw:                      raw,
	}
	return reading, "", true
}

// mapHourlyValue converts one hourly_values valueList entry into an
// integration.HourlyValue. Unlike mapEnergyRow, this is never multiplied:
// 06 §2 describes hourly_values as "already-differenced consumption
// values", a provider-computed cross-check series kept separate from the
// multiplied index readings (removed-behaviour 23), not a register that
// R8's multiply-once rule applies to.
func mapHourlyValue(v wireHourlyValue) (hv integration.HourlyValue, field string, ok bool) {
	ts, err := normalize.OSOSDate(v.MeterDate)
	if err != nil {
		return integration.HourlyValue{}, "meter_date", false
	}
	consumption, err := normalize.OptionalNumber(v.ActiveConsumption)
	if err != nil {
		return integration.HourlyValue{}, "activeConsumption", false
	}
	generation, err := normalize.OptionalNumber(v.ActiveGeneration)
	if err != nil {
		return integration.HourlyValue{}, "activeGeneration", false
	}
	return integration.HourlyValue{Ts: ts, ActiveConsumption: consumption, ActiveGeneration: generation}, "", true
}
