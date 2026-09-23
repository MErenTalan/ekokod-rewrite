package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
)

func every(v string) domain.MonthSeries {
	var s domain.MonthSeries
	for i := range s {
		d := decimal.RequireFromString(v)
		s[i] = &d
	}
	return s
}

// subset requires every key of want to be in got with the same value; got
// may carry more (false booleans the domain omits).
func subset(t *testing.T, path string, want, got any) {
	t.Helper()
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		require.True(t, ok, "%s: not an object in the DTO", path)
		for k, v := range w {
			gv, ok := g[k]
			require.True(t, ok, "%s.%s is lost on the wire", path, k)
			subset(t, path+"."+k, v, gv)
		}
	case []any:
		g, ok := got.([]any)
		require.True(t, ok, "%s: not an array in the DTO", path)
		require.Len(t, g, len(w), path)
		for i := range w {
			subset(t, path, w[i], g[i])
		}
	default:
		if arr, ok := got.([]any); ok && want == nil && len(arr) == 0 {
			return // a null array is sent as [] (TestReportPayloadHasNoNullArrays)
		}
		require.Equal(t, want, got, path)
	}
}

func roundTrip(t *testing.T, p domain.Payload) {
	t.Helper()
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	out, err := payloadDTO(p)
	require.NoError(t, err)
	wire, err := json.Marshal(out)
	require.NoError(t, err)
	var want, got any
	require.NoError(t, json.Unmarshal(raw, &want))
	require.NoError(t, json.Unmarshal(wire, &got))
	subset(t, "payload", want, got)
}

func TestReportPayloadDTOKeepsEveryField(t *testing.T) {
	t.Parallel()
	price := decimal.RequireFromString("3.1")
	name := "Sanayi"
	bills := [12]*domain.Bill{}
	for i := range bills {
		bills[i] = &domain.Bill{Currency: "TRY", Total: decimal.NewFromInt(100), EnergyCost: decimal.NewFromInt(60),
			NetConsumption: decimal.NewFromInt(20), EffectivePrice: &price, GenerationPrice: &price}
	}
	b := domain.BuildingInput{ID: uuid.New(), Name: "B", TariffName: &name,
		Consumption: map[int]domain.MonthSeries{2025: every("10"), 2026: every("12")}, Export: map[int]domain.MonthSeries{2026: every("1")},
		Partial: map[int][12]bool{2026: {true}}, T1: &price, T2: &price, T3: &price, Inductive: &price, Capacitive: &price,
		Bills: map[int][12]*domain.Bill{2025: bills, 2026: bills}}
	target := decimal.NewFromInt(500)
	plants := []domain.PlantInput{{ID: uuid.New(), Name: "P", Kind: domain.KindGrid, FeedIn: &price, YearlyTarget: &target,
		Production: map[int]domain.MonthSeries{2026: every("5")}}}
	m := domain.BuildMonthly(domain.MonthlyInput{Year: 2026, Month: 1, Selection: domain.SelectionAll, Buildings: []domain.BuildingInput{b}, Plants: plants})
	roundTrip(t, domain.Payload{Version: 1, Type: domain.TypeMonthly, Period: "2026-01", Monthly: &m})
	year := 2022
	y := domain.BuildYearly(domain.YearlyInput{Year: 2026, Selection: domain.SelectionAll, Buildings: []domain.BuildingInput{b}, Plants: plants,
		Factor: &domain.GridFactor{Value: price, Unit: "kg", SourceYear: &year}})
	roundTrip(t, domain.Payload{Version: 1, Type: domain.TypeYearly, Period: "2026", Yearly: &y})
	y.Carbon, y.CarbonReason = nil, "grid_factor_missing"
	roundTrip(t, domain.Payload{Version: 1, Type: domain.TypeYearly, Period: "2026", Yearly: &y})
}

// requireNoNilSlices walks v and fails on any nil slice: the contract marks
// the payload's arrays required, and a screen reading `.length` of a null
// crashes (found by the F8b e2e run on a year without invoices).
func requireNoNilSlices(t *testing.T, path string, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			requireNoNilSlices(t, path, v.Elem())
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				requireNoNilSlices(t, path+"."+v.Type().Field(i).Name, v.Field(i))
			}
		}
	case reflect.Slice:
		require.False(t, v.IsNil(), "%s is null on the wire", path)
		for i := range v.Len() {
			requireNoNilSlices(t, path, v.Index(i))
		}
	}
}

func TestReportPayloadHasNoNullArrays(t *testing.T) {
	t.Parallel()
	empty := domain.BuildingInput{ID: uuid.New(), Name: "Boş"}
	m := domain.BuildMonthly(domain.MonthlyInput{Year: 2026, Month: 1, Selection: domain.SelectionAll, Buildings: []domain.BuildingInput{empty}})
	y := domain.BuildYearly(domain.YearlyInput{Year: 2026, Selection: domain.SelectionAll, Buildings: []domain.BuildingInput{empty}})
	for _, p := range []domain.Payload{
		{Version: 1, Type: domain.TypeMonthly, Period: "2026-01", Monthly: &m},
		{Version: 1, Type: domain.TypeYearly, Period: "2026", Yearly: &y},
	} {
		out, err := payloadDTO(p)
		require.NoError(t, err)
		requireNoNilSlices(t, "payload", reflect.ValueOf(out))
		stored, err := json.Marshal(p)
		require.NoError(t, err)
		decoded, err := storedPayloadDTO(stored)
		require.NoError(t, err)
		requireNoNilSlices(t, "stored", reflect.ValueOf(decoded))
	}
}
