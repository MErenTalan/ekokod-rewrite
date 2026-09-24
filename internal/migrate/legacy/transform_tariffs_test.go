package legacy_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

const building2Hex = "64f000000000000000000b02"

func price(fields ...any) bson.D {
	d := bson.D{{Key: "distribution_cost", Value: 1.0}, {Key: "reactive_power_price", Value: 0.5}}
	for i := 0; i+1 < len(fields); i += 2 {
		d = append(d, bson.E{Key: fields[i].(string), Value: fields[i+1]})
	}
	return d
}

func embedded(from, priceType string, p bson.D, extra ...bson.E) bson.D {
	return append(bson.D{{Key: "effectiveFrom", Value: from}, {Key: "currency", Value: "tl"}, {Key: "price_type", Value: priceType}, {Key: "price", Value: p}}, extra...)
}

// tariffSource extends the transform fixture with R417–R420's collections.
func tariffSource(t *testing.T) *legacy.MemSource {
	src := transformSource(t)
	src.Replace("buildings", tariffBuildings(nil)...)
	src.Add("tariffs",
		bson.D{{Key: "_id", Value: oid("64f000000000000000001a01")}, {Key: "building", Value: oid(buildingHex)}, {Key: "effectiveFrom", Value: "01-01-2025"},
			{Key: "currency", Value: "tl"}, {Key: "energy_type", Value: "grid_energy"}, {Key: "distribution_type", Value: "ag"},
			{Key: "distribution_system_user", Value: "commercial"}, {Key: "price_type", Value: "single_time"}, {Key: "term", Value: "monomial"},
			{Key: "supply_company", Value: "attendant_company"}, {Key: "price", Value: price("single_time_price", 2.4)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000001a02")}, {Key: "effectiveFrom", Value: "01-01-2025"}, {Key: "currency", Value: "tl"}},
		// As many price fields as the embedded 2025-07-01 record: a tie, which the embedded record wins.
		bson.D{{Key: "_id", Value: oid("64f000000000000000001a03")}, {Key: "building", Value: oid(buildingHex)}, {Key: "effectiveFrom", Value: "2025-07-01"},
			{Key: "currency", Value: "tl"}, {Key: "price_type", Value: "single_time"}, {Key: "price", Value: price("single_time_price", 2.9)}},
	)
	tpl := embedded("01-01-2024", "single_time", price("single_time_price", 3.0, "vat_rate", 20.0))
	src.Add("tarifftemplates",
		bson.D{{Key: "_id", Value: oid("64f000000000000000002a01")}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "Konut 2024"}, {Key: "tariff", Value: tpl}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000002a02")}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "konut 2024 "}, {Key: "tariff", Value: tpl}, {Key: "isDefault", Value: true}},
	)
	smtp := func(hex, host string) bson.D {
		return bson.D{{Key: "_id", Value: oid(hex)}, {Key: "company", Value: oid(companyHex)}, {Key: "host", Value: host}, {Key: "port", Value: int32(587)},
			{Key: "auth", Value: bson.D{{Key: "user", Value: "mailer"}, {Key: "pass", Value: vecPrimary}}}, {Key: "from", Value: "enerji@acme.test"}}
	}
	src.Add("smtpsettings", smtp("64f000000000000000003a01", "old.acme.test"), smtp("64f000000000000000003a02", "smtp.acme.test"))
	src.Add("epiashistories",
		bson.D{{Key: "_id", Value: oid("64f000000000000000004a01")}, {Key: "date", Value: "2026-08-01"}, {Key: "hour", Value: int32(0)}, {Key: "ptf", Value: 2000.0}, {Key: "yekdem", Value: 300.0}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000004a02")}, {Key: "date", Value: "2026-08-01"}, {Key: "hour", Value: int32(1)}, {Key: "ptf", Value: 2100.0}, {Key: "yekdem", Value: 300.0}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000004a03")}, {Key: "date", Value: "2026-08-02"}, {Key: "ptfAvg", Value: 2050.0}, {Key: "yekdem", Value: 310.0}},
	)
	return src
}

func transformWith(t *testing.T, src *legacy.MemSource, answers map[string]map[string]string) (string, legacy.TransformResult, *crypto.Cipher) {
	t.Helper()
	extract := t.TempDir()
	_, err := legacy.Extract(context.Background(), src, extract)
	require.NoError(t, err)
	return transformFrom(t, extract, answers)
}

func transformFrom(t *testing.T, extract string, answers map[string]map[string]string) (string, legacy.TransformResult, *crypto.Cipher) {
	t.Helper()
	cipher, err := crypto.NewCipher(make([]byte, 32))
	require.NoError(t, err)
	out := t.TempDir()
	res, err := legacy.Transform(extract, out, legacy.TransformOptions{Keys: legacy.Keys{Primary: "legacy-secret-key"}, Cipher: cipher, Now: now, Answers: answers})
	require.NoError(t, err)
	return out, res, cipher
}

func byID(rows []map[string]any, id string) map[string]any {
	for _, r := range rows {
		if r["id"] == id || r["tariff_id"] == id {
			return r
		}
	}
	return nil
}

func rejectReasons(t *testing.T, dir string) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "rejects.ndjson"))
	require.NoError(t, err)
	out := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &r))
		out[r["reason"].(string)] = append(out[r["reason"].(string)], r["legacy_id"].(string))
	}
	return out
}

func TestTariffsAreMergedClassifiedAndValidated(t *testing.T) {
	out, res, _ := transformWith(t, tariffSource(t), nil)
	tariffs := rows(t, out, "tariffs")
	first := byID(tariffs, legacy.ID("building_tariffs", buildingHex+":0").String())
	require.NotNil(t, first, "the embedded record has more price fields than the standalone one: it wins (08 §5)")
	require.Equal(t, "2025-01-01", first["effective_from"])
	require.Equal(t, "2.5", first["single_time_price"], "legacy priced energy as single_time_price || power_price")
	require.Equal(t, []any{"lv", "commercial", "monomial", "incumbent"}, []any{first["voltage_level"], first["user_group"], first["term"], first["supply_company"]},
		"classification from the standalone record of the same date (Q-J7)")
	require.Nil(t, first["contracted_power_kw"], "R125: a monomial tariff has no power charge")
	require.Equal(t, 1, res.Notes["monomial_power_dropped"])
	tax := byID(rows(t, out, "tariff_taxes"), first["id"].(string))
	require.Equal(t, "Diğer vergi ve fonlar", tax["name"])
	require.Equal(t, "3.35", tax["rate"])

	ptf := byID(tariffs, legacy.ID("building_tariffs", buildingHex+":3").String())
	require.NotNil(t, ptf)
	require.Equal(t, "2026-03-01", ptf["effective_from"])
	require.Equal(t, []any{"1.1", "kbk", "kbk", "fixed"}, []any{ptf["kbk_energy"], ptf["reactive_price_source"], ptf["distribution_price_source"], ptf["power_price_source"]})
	require.Equal(t, true, ptf["use_manual_yekdem"])
	yekdem := byID(rows(t, out, "tariff_manual_yekdem"), ptf["id"].(string))
	require.Equal(t, "1500", yekdem["value"])

	reasons := rejectReasons(t, out)
	require.Equal(t, []string{"64f000000000000000001a01", "64f000000000000000001a03"}, reasons["duplicate_merged"], "ties go to the embedded record")
	require.Equal(t, []string{buildingHex + ":1"}, reasons["vat_rate_missing"])
	require.Equal(t, []string{buildingHex + ":2"}, reasons["kbk_energy_missing"])
	require.Equal(t, []string{building2Hex + ":0"}, reasons["classification_unknown"], "never guessed (Q-J7)")
	require.Equal(t, []string{"64f000000000000000001a02"}, reasons["no_building"], "Q-J8")
	require.Equal(t, legacy.Tally{Accepted: 2, Rejected: 3}, res.Summary["building_tariffs"])
	require.Equal(t, 1, res.Asked[legacy.AnswerTariffClass])
	manual, err := os.ReadFile(filepath.Join(out, "manual_tariff_class.csv"))
	require.NoError(t, err)
	require.Contains(t, string(manual), building2Hex)
}

func TestAnAnsweredClassificationIsUsed(t *testing.T) {
	out, res, _ := transformWith(t, tariffSource(t), map[string]map[string]string{legacy.AnswerTariffClass: {building2Hex: "og/industrial/binomial/private"}})
	depo := byID(rows(t, out, "tariffs"), legacy.ID("building_tariffs", building2Hex+":0").String())
	require.NotNil(t, depo)
	require.Equal(t, []any{"mv", "industrial", "binomial", "private", "50"}, []any{depo["voltage_level"], depo["user_group"], depo["term"], depo["supply_company"], depo["contracted_power_kw"]})
	require.Zero(t, res.Asked[legacy.AnswerTariffClass])
}

func TestTemplatesSMTPAndMarketPrices(t *testing.T) {
	out, res, cipher := transformWith(t, tariffSource(t), nil)

	templates := rows(t, out, "tariff_templates")
	require.Len(t, templates, 1, "same company and name, case-folded: the last wins (R418)")
	require.Equal(t, "konut 2024", templates[0]["name"])
	require.Equal(t, true, templates[0]["is_default"])
	raw, err := json.Marshal(templates[0]["payload"])
	require.NoError(t, err)
	var in tariffsvc.Input
	require.NoError(t, json.Unmarshal(raw, &in), "the payload is what ApplyTemplate decodes")
	require.Equal(t, model.VoltageLevelLV, in.Tariff.VoltageLevel)
	require.Equal(t, "3", in.Tariff.SingleTimePrice.String())
	require.Equal(t, 1, res.Asked[legacy.AnswerTariffTemplateClass], "only the kept template is asked about")
	require.Contains(t, rejectReasons(t, out)["duplicate_name"], "64f000000000000000002a01")

	smtp := rows(t, out, "smtp_settings")
	require.Len(t, smtp, 1)
	require.Equal(t, "smtp.acme.test", smtp[0]["host"])
	company := legacy.ID("companies", companyHex)
	opened, err := cipher.Open(smtp[0]["password_enc"].(string), postgres.SMTPPasswordAAD(company))
	require.NoError(t, err)
	require.Equal(t, "Şifre!2024", string(opened))

	prices := rows(t, out, "market_prices_hourly")
	require.Equal(t, []any{"2026-07-31T21:00:00Z", "2026-07-31T22:00:00Z"}, []any{prices[0]["ts"], prices[1]["ts"]}, "Istanbul hours, stored UTC")
	require.Equal(t, []map[string]any{{"year": float64(2026), "month": float64(8), "value": "310"}}, rows(t, out, "yekdem_monthly"), "the latest date's YEKDEM")
	require.Equal(t, 1, res.Notes["yekdem_conflicts"])
	require.Equal(t, legacy.Tally{Accepted: 3}, res.Summary["epiashistories"])
}

// tariffBuildings are the two buildings with R417's tariff history (and, for
// the history tests, a bill history on the first).
func tariffBuildings(bills bson.D) []bson.D {
	kbk := bson.D{{Key: "energyKbk", Value: 1.1}, {Key: "t1Kbk", Value: 1.0}, {Key: "t2Kbk", Value: 1.2}, {Key: "t3Kbk", Value: 0.8},
		{Key: "reactivePowerKbk", Value: 1.0}, {Key: "distributionCostTlPerKwh", Value: 0.9}, {Key: "useManualYekdem", Value: true},
		{Key: "manualYekdem", Value: bson.A{bson.D{{Key: "year", Value: int32(2026)}, {Key: "month", Value: int32(3)}, {Key: "value", Value: 1500.0}}}}}
	return []bson.D{
		{{Key: "_id", Value: oid(buildingHex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "Merkez"}, {Key: "billHistory", Value: bills}, {Key: "user_in_charge", Value: oid("64f000000000000000000d01")},
			{Key: "tariffs", Value: bson.A{
				embedded("01-01-2025", "single_time", price("power_price", 2.5, "vat_rate", 20.0, "other_taxes_rate", 3.35, "contracted_power", 100.0, "power_unit_price", 10.0)),
				embedded("2025-07-01", "single_time", price("single_time_price", 2.6)),
				embedded("2026-01-01", "single_time", price("single_time_price", 2.7, "vat_rate", 20.0), bson.E{Key: "usePtfYekdem", Value: true}),
				embedded("01-03-2026", "multi_time", price("vat_rate", 20.0, "allTaxes", bson.A{bson.D{{Key: "name", Value: "BTV"}, {Key: "rate", Value: 1.0}}}),
					bson.E{Key: "usePtfYekdem", Value: true}, bson.E{Key: "kbk", Value: kbk}),
			}}},
		{{Key: "_id", Value: oid(building2Hex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "Depo"},
			{Key: "tariffs", Value: bson.A{embedded("2026-01-01", "single_time", price("single_time_price", 3.0, "vat_rate", 20.0, "contracted_power", 50.0, "power_unit_price", 12.0))}}},
	}
}
