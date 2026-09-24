package legacy_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

var now = time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

func ososRow(date, top string) bson.D {
	return bson.D{{Key: "meter_date", Value: date}, {Key: "meter_serial_no", Value: "S1"}, {Key: "calculation_type", Value: int32(0)},
		{Key: "t_top_kWh", Value: top}, {Key: "t_ri_kVarh", Value: "1"}, {Key: "t_rc_kVarh", Value: "0"}, {Key: "t_t1_kWh", Value: "0"},
		{Key: "t_t2_kWh", Value: "0"}, {Key: "t_t3_kWh", Value: "0"}, {Key: "t_p_kW", Value: "0"}, {Key: "u_top_kWh", Value: ""},
		{Key: "u_ri_kVarh", Value: ""}, {Key: "u_rc_kVarh", Value: ""}, {Key: "u_u1_kWh", Value: ""}, {Key: "u_u2_kWh", Value: ""},
		{Key: "u_u3_kWh", Value: ""}, {Key: "u_p_kW", Value: ""}}
}

func analyzerDoc(sub string, mult string, lp ...bson.D) bson.M {
	rows := bson.A{}
	for _, r := range lp {
		rows = append(rows, r)
	}
	return bson.M{"_id": oid("64f0000000000000000000a1"), "subIntegration": sub, "meterMultiplier": mult,
		"energyValues": bson.M{"loadProfile": rows, "daily": bson.A{ososRow("23/09/2026 00:00:00", "90")}}}
}

func TestMultiplierDecisions(t *testing.T) {
	for _, c := range []struct {
		provider, stored, confirmed string
		determined                  bool
		mapWith, applied            string
	}{
		{"ARIL", "40", "", true, "1", "40"},     // legacy stored ARIL values already multiplied
		{"GRIDBOX", "40", "", true, "40", "40"}, // provider-multiplied register or raw × m
		{"OSOS", "", "", true, "1", "1"},        // no multiplier: nothing to decide
		{"OSOS", "1", "", true, "1", "1"},
		{"OSOS", "40", "", false, "1", "1"},     // cannot know whether the provider pre-multiplied (Q-J5)
		{"OSOS", "40", "raw", true, "40", "40"}, // an operator confirmed raw values
		{"OSOS", "40", "multiplied", true, "1", "40"},
		{"PM5340", "1", "", false, "1", "1"},
	} {
		d := legacy.DecideMultiplier(c.provider, c.stored, c.confirmed)
		require.Equal(t, c.determined, d.Determined, "%+v", c)
		require.True(t, decimal.RequireFromString(c.mapWith).Equal(d.MapWith), "%+v map %s", c, d.MapWith)
		require.True(t, decimal.RequireFromString(c.applied).Equal(d.Applied), "%+v applied %s", c, d.Applied)
	}
}

func TestAnalyzerReadingsFromEveryArray(t *testing.T) {
	doc := analyzerDoc("Baskent", "", ososRow("24/09/2026 10:00:00", "100"), ososRow("24/09/2026 11:00:00", "101"))
	res := legacy.AnalyzerReadings(doc, "OSOS", legacy.DecideMultiplier("OSOS", "", ""), now)
	require.Len(t, res.Readings, 3)
	kinds := map[model.ReadingKind]int{}
	for _, r := range res.Readings {
		kinds[r.Kind]++
		require.Equal(t, legacy.ID("analyzers", "64f0000000000000000000a1"), r.AnalyzerID)
	}
	require.Equal(t, map[model.ReadingKind]int{model.ReadingKindLoadProfile: 2, model.ReadingKindDaily: 1}, kinds)
	require.Empty(t, res.Rejects)
}

func TestDuplicatesKeepTheLastAndAreCounted(t *testing.T) {
	doc := analyzerDoc("Baskent", "", ososRow("24/09/2026 10:00:00", "100"), ososRow("24/09/2026 10:00:00", "105"))
	res := legacy.AnalyzerReadings(doc, "OSOS", legacy.DecideMultiplier("OSOS", "", ""), now)
	require.Equal(t, 1, res.Duplicates)
	var lp []model.MeterReading
	for _, r := range res.Readings {
		if r.Kind == model.ReadingKindLoadProfile {
			lp = append(lp, r)
		}
	}
	require.Len(t, lp, 1)
	require.True(t, decimal.NewFromInt(105).Equal(*lp[0].ActiveImport), "last wins (R408)")
}

func TestBadAndOutOfRangeRowsAreRejectedWithReasons(t *testing.T) {
	doc := analyzerDoc("Baskent", "", ososRow("99/99/2026 10:00:00", "1"), ososRow("24/09/2027 10:00:00", "1"), ososRow("01/01/2009 00:00:00", "1"))
	res := legacy.AnalyzerReadings(doc, "OSOS", legacy.DecideMultiplier("OSOS", "", ""), now)
	reasons := map[string]int{}
	for _, r := range res.Rejects {
		reasons[r.Reason]++
	}
	require.Equal(t, map[string]int{"unparseable:meter_date": 1, "out_of_range": 2}, reasons)
	require.Equal(t, 3+1, len(res.Rejects)+len(res.Readings), "every row is accepted or rejected")
}

func TestNegativeIndexDeltasBecomeAnomalies(t *testing.T) {
	doc := analyzerDoc("Baskent", "", ososRow("24/09/2026 10:00:00", "100"), ososRow("24/09/2026 11:00:00", "40"), ososRow("24/09/2026 12:00:00", "41"))
	res := legacy.AnalyzerReadings(doc, "OSOS", legacy.DecideMultiplier("OSOS", "", ""), now)
	require.Len(t, res.Drops, 1)
	d := res.Drops[0]
	require.Equal(t, model.ReadingKindLoadProfile, d.Kind)
	require.Equal(t, time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC), d.To.UTC())
	require.True(t, decimal.NewFromInt(-60).Equal(d.Delta))
}

func TestArilZerosAreNullAndValuesAreNotMultipliedTwice(t *testing.T) {
	doc := analyzerDoc("Aril", "40", ososRow("24/09/2026 10:00:00", "4000"))
	res := legacy.AnalyzerReadings(doc, "ARIL", legacy.DecideMultiplier("ARIL", "40", ""), now)
	var r model.MeterReading
	for _, x := range res.Readings {
		if x.Kind == model.ReadingKindLoadProfile {
			r = x
		}
	}
	require.True(t, decimal.NewFromInt(4000).Equal(*r.ActiveImport))
	require.True(t, decimal.NewFromInt(40).Equal(r.MultiplierApplied))
	require.Equal(t, model.IntegrationProviderARIL, r.SourceProvider)
	require.Nil(t, r.T1Import, "ARIL wrote \"0\" for T1–T3 (06 §4)")
	require.Nil(t, r.T2Import)
	require.Nil(t, r.T3Import)
	require.Nil(t, r.MaxDemandKw)
}

func TestGridboxRowsUseTheProviderMultipliedRegister(t *testing.T) {
	doc := bson.M{"_id": oid("64f0000000000000000000a2"), "subIntegration": "GB", "meterMultiplier": "40",
		"energyValues": bson.M{"loadProfile": bson.A{bson.D{{Key: "ProfileDateTime", Value: "2026-09-24T13:00:00+03:00"}, {Key: "ActiveEndex", Value: int32(100)}, {Key: "ActiveEndexWithMultiplier", Value: int32(4000)}}}}}
	res := legacy.AnalyzerReadings(doc, "GRIDBOX", legacy.DecideMultiplier("GRIDBOX", "40", ""), now)
	require.Len(t, res.Readings, 1)
	require.True(t, decimal.NewFromInt(4000).Equal(*res.Readings[0].ActiveImport))
}

// Documents read back from an extract carry bson.D sub-documents, not bson.M.
func TestReadingsFromAnExtractedDocument(t *testing.T) {
	src := legacy.NewMemSource()
	src.Add("analyzers", bson.D{{Key: "_id", Value: oid("64f0000000000000000000a1")}, {Key: "subIntegration", Value: "Baskent"},
		{Key: "energyValues", Value: bson.D{{Key: "loadProfile", Value: bson.A{ososRow("24/09/2026 10:00:00", "100")}}}}})
	dir := t.TempDir()
	_, err := legacy.Extract(context.Background(), src, dir)
	require.NoError(t, err)
	var got int
	require.NoError(t, legacy.ReadExtract(dir, "analyzers", func(doc bson.M) error {
		got = len(legacy.AnalyzerReadings(doc, "OSOS", legacy.DecideMultiplier("OSOS", "", ""), now).Readings)
		return nil
	}))
	require.Equal(t, 1, got)
}
