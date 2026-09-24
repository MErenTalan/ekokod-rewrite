package legacy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

const user4Hex = "64f000000000000000000d04"

func carbonISOSource(t *testing.T) *legacy.MemSource {
	src := fullSource(t)
	src.Add("users", bson.D{{Key: "_id", Value: oid(user4Hex)}, {Key: "name", Value: "Z"}, {Key: "email", Value: "z@acme.test"}, {Key: "password", Value: bcryptHash},
		{Key: "userType", Value: "company-admin"}, {Key: "company", Value: oid(companyHex)}})
	src.Replace("carbonfootprint", bson.D{{Key: "_id", Value: oid("64f000000000000000000e01")}, {Key: "company_id", Value: oid(companyHex)},
		{Key: "building_id", Value: oid(buildingHex)}, {Key: "selectedActivities", Value: bson.A{"sub_waste_disposal", "sub_bogus"}},
		{Key: "activities", Value: bson.A{
			bson.D{{Key: "_id", Value: oid("64f000000000000000009a01")}, {Key: "date", Value: "01-08-2026"}, {Key: "endDate", Value: "31-08-2026"},
				{Key: "mainCategory", Value: "Atık"}, {Key: "subCategory", Value: "sub_waste_disposal"}, {Key: "type", Value: "sub_waste_disposal"},
				{Key: "amount", Value: 2.0}, {Key: "unit", Value: "tonne"}, {Key: "emissionCo2e", Value: 1000.1234567}, {Key: "status", Value: "Onaylandı"},
				{Key: "factorKey", Value: "custom_x"}, {Key: "emissionFactorUnitValue", Value: 500.0}, {Key: "scope", Value: "scope_1"}},
			bson.D{{Key: "date", Value: "2026-09-01"}, {Key: "endDate", Value: "2026-09-01"}, {Key: "subCategory", Value: "sub_grid_electricity"},
				{Key: "amount", Value: 120.0}, {Key: "unit", Value: "kWh"}, {Key: "emissionCo2e", Value: 120.0}, {Key: "description", Value: "Günlük tüketim otomatik olarak eklenmiştir"}},
			bson.D{{Key: "date", Value: "2026-09-01"}, {Key: "endDate", Value: "2026-09-01"}, {Key: "subCategory", Value: "sub_nope"}, {Key: "amount", Value: 1.0},
				{Key: "unit", Value: "kg"}, {Key: "emissionCo2e", Value: 1.0}},
		}}})
	src.Add("companyemissionfactors", bson.D{{Key: "_id", Value: oid("64f000000000000000009b01")}, {Key: "company_id", Value: oid(companyHex)},
		{Key: "emissionFactors", Value: bson.A{
			bson.D{{Key: "key", Value: "stationary_space_heating_coal_domestic"}, {Key: "mainCategory", Value: "cat_stationary"}, {Key: "baseFactor", Value: 2.904}, {Key: "baseUnit", Value: "tonne"}},
			bson.D{{Key: "key", Value: "custom_x"}, {Key: "label", Value: "Özel atık"}, {Key: "mainCategory", Value: "cat_waste"}, {Key: "subCategory", Value: bson.A{"sub_waste_disposal"}},
				{Key: "baseFactor", Value: 1.5}, {Key: "baseUnit", Value: "kg"}, {Key: "conversion", Value: bson.A{bson.D{{Key: "unit", Value: "g"}, {Key: "multiplier", Value: 0.001}, {Key: "label", Value: "gram"}}}},
				{Key: "metadata", Value: bson.D{{Key: "source", Value: "Firma"}, {Key: "year", Value: int32(2025)}}}},
		}}})
	src.Add("carbonreporthistories", bson.D{{Key: "_id", Value: oid("64f000000000000000009c01")}, {Key: "companyID", Value: oid(companyHex)}, {Key: "buildingID", Value: oid(buildingHex)},
		{Key: "reports", Value: bson.A{bson.D{{Key: "_id", Value: oid("64f000000000000000009c02")}, {Key: "name", Value: "GHG 2025"}, {Key: "storedFileName", Value: "ghg.pdf"},
			{Key: "createdDate", Value: "2026-01-05"}, {Key: "datePeriod", Value: "2025"}, {Key: "reportType", Value: "ghg"}, {Key: "pdfPath", Value: "/opt/documents/ghg.pdf"}}}}})
	src.Add("projectDates", bson.D{{Key: "_id", Value: oid("64f00000000000000000aa01")}, {Key: "userId", Value: "64f000000000000000000d01"},
		{Key: "dates", Value: bson.D{{Key: "5", Value: bson.D{{Key: "startDate", Value: "2026-01-01"}, {Key: "endDate", Value: "2026-03-31"}}},
			{Key: "12", Value: bson.D{{Key: "startDate", Value: "2026-01-01"}}}}}})
	src.Add("notes",
		bson.D{{Key: "_id", Value: oid("64f00000000000000000ab01")}, {Key: "userId", Value: "64f000000000000000000d01"}, {Key: "subItem", Value: "5.1"},
			{Key: "title", Value: "Taahhüt"}, {Key: "text", Value: "Genel müdür imzaladı."}},
		bson.D{{Key: "_id", Value: oid("64f00000000000000000ab02")}, {Key: "userId", Value: user4Hex}, {Key: "subItem", Value: "5.1"}, {Key: "text", Value: "Belirsiz"}},
	)
	src.Add("files", bson.D{{Key: "_id", Value: oid("64f00000000000000000ac01")}, {Key: "userId", Value: "64f000000000000000000d01"}, {Key: "itemNo", Value: "5.1"},
		{Key: "originalName", Value: "Politika.pdf"}, {Key: "path", Value: "/uploads/64f000000000000000000d01/5.1/1-politika.pdf"}})
	return src
}

func TestCarbonActivitiesFactorsAndReports(t *testing.T) {
	out, res, _ := transformWith(t, carbonISOSource(t), nil)
	acts := rows(t, out, "carbon_activities")
	require.Len(t, acts, 1, "manual records only: the daily accrual is recomputed (08 §1)")
	a := acts[0]
	require.Equal(t, []any{"cat_waste", "sub_waste_disposal", "scope_3", "category_6", "approved", "1000.123457", "2026-08-01", "2026-08-31"},
		[]any{a["main_category"], a["sub_category"], a["scope"], a["iso_category"], a["status"], a["emission_kgco2e"], a["period_start"], a["period_end"]},
		"scope and ISO category from the sub-category (R301), never the stored ones")
	reasons := rejectReasons(t, out)
	require.Len(t, reasons["automated_accrual_recomputed"], 1)
	require.Contains(t, reasons["unknown_sub_category"], "64f000000000000000000e01:sub_bogus")
	require.Equal(t, []map[string]any{{"company_id": legacy.ID("companies", companyHex).String(), "building_id": legacy.ID("buildings", buildingHex).String(), "activity_key": "sub_waste_disposal"}},
		rows(t, out, "carbon_selected_activities"))

	factors := rows(t, out, "emission_factors")
	require.Len(t, factors, 1, "a copy of the master factor is not an override")
	require.Equal(t, []any{"custom_x", "1.5", "Firma", float64(2025)}, []any{factors[0]["key"], factors[0]["base_factor"], factors[0]["source"], factors[0]["source_year"]})
	require.Equal(t, 1, res.Notes["factor_same_as_master"])
	require.Len(t, rows(t, out, "emission_factor_conversions"), 1)

	reports := rows(t, out, "carbon_reports")
	require.Len(t, reports, 1)
	require.Equal(t, []any{"GHG 2025", "ghg", "2025", "/opt/documents/ghg.pdf"}, []any{reports[0]["name"], reports[0]["report_type"], reports[0]["period"], reports[0]["pdf_path"]})
}

func TestISOContentMovesToTheUsersBuilding(t *testing.T) {
	out, res, _ := transformWith(t, carbonISOSource(t), nil)
	project := legacy.ID("iso50001_projects", buildingHex).String()
	require.Equal(t, []map[string]any{{"id": project, "company_id": legacy.ID("companies", companyHex).String(), "building_id": legacy.ID("buildings", buildingHex).String()}},
		rows(t, out, "iso50001_projects"), "the building the user is in charge of (Q-J9)")
	require.Equal(t, []map[string]any{{"project_id": project, "clause_id": "5", "start_date": "2026-01-01", "end_date": "2026-03-31"}}, rows(t, out, "iso50001_clause_dates"))
	notes := rows(t, out, "iso50001_notes")
	require.Len(t, notes, 1)
	require.Equal(t, []any{"5.1", "Taahhüt", "Genel müdür imzaladı."}, []any{notes[0]["clause_id"], notes[0]["title"], notes[0]["body"]})
	reasons := rejectReasons(t, out)
	require.Contains(t, reasons["unknown_clause"], "64f00000000000000000aa01:12")
	require.Equal(t, []string{"64f00000000000000000ab02"}, reasons["building_ambiguous"], "two buildings, none in the user's charge: asked, never guessed")
	require.Equal(t, 1, res.Asked[legacy.AnswerISOBuilding])

	refs := rows(t, out, "artifact_refs")
	var iso map[string]any
	for _, r := range refs {
		if r["owner_type"] == "iso50001" {
			iso = r
		}
	}
	require.Equal(t, []any{"5.1", "Politika.pdf", legacy.ID("buildings", buildingHex).String()}, []any{iso["clause_id"], iso["original_name"], iso["owner_id"]})

	answered, res2, _ := transformWith(t, carbonISOSource(t), map[string]map[string]string{legacy.AnswerISOBuilding: {user4Hex: building2Hex}})
	require.Len(t, rows(t, answered, "iso50001_projects"), 2)
	require.Zero(t, res2.Asked[legacy.AnswerISOBuilding])
}
