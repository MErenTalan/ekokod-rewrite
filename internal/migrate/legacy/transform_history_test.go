package legacy_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

func historySource(t *testing.T) *legacy.MemSource {
	src := tariffSource(t)
	bills := bson.D{
		{Key: "2026-08", Value: bson.D{{Key: "totalCost", Value: 1000.5}, {Key: "totalActiveKWh", Value: "1.234,5"}, {Key: "reactivePenaltyApplied", Value: true},
			{Key: "startDate", Value: "01-08-2026"}, {Key: "endDate", Value: "31-08-2026"}, {Key: "pdfPath", Value: "/opt/bills/b-2026-08.pdf"}}},
		{Key: "2026-13", Value: bson.D{{Key: "totalCost", Value: 1.0}}},
	}
	src.Replace("buildings", tariffBuildings(bills)...)
	src.Add("reports",
		bson.D{{Key: "_id", Value: oid("64f000000000000000005a01")}, {Key: "company", Value: oid(companyHex)}, {Key: "building", Value: oid(buildingHex)},
			{Key: "reportType", Value: "monthly"}, {Key: "period", Value: "2026-08"}, {Key: "monthlyReport", Value: bson.D{{Key: "month", Value: "2026-08"}}},
			{Key: "pdfPath", Value: "/opt/reports/r.pdf"}, {Key: "excelPath", Value: "/opt/reports/r.xlsx"}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000005a02")}, {Key: "company", Value: oid("64f000000000000000000cff")}, {Key: "reportType", Value: "yearly"}, {Key: "period", Value: "2025"}},
	)
	at := func(d time.Duration) bson.DateTime { return bson.NewDateTimeFromTime(now.Add(-d)) }
	src.Add("logs",
		bson.D{{Key: "_id", Value: oid("64f000000000000000006a01")}, {Key: "type", Value: "cron"}, {Key: "category", Value: "bill-generation"},
			{Key: "message", Value: "Fatura oluşturuldu"}, {Key: "status", Value: "success"}, {Key: "relatedModel", Value: "Analyzer"},
			{Key: "relatedId", Value: "64f0000000000000000000a1"}, {Key: "timestamp", Value: at(24 * time.Hour)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000006a02")}, {Key: "type", Value: "alarm"}, {Key: "category", Value: "alarm-trigger"},
			{Key: "message", Value: "Eski"}, {Key: "status", Value: "info"}, {Key: "timestamp", Value: at(400 * 24 * time.Hour)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000006a03")}, {Key: "type", Value: "system"}, {Key: "category", Value: "x"},
			{Key: "message", Value: "Bilinmeyen"}, {Key: "status", Value: "info"}, {Key: "relatedModel", Value: "Alarm"}, {Key: "relatedId", Value: "zz"},
			{Key: "timestamp", Value: at(time.Hour)}},
	)
	return src
}

func TestBillHistoryReportsAndLogs(t *testing.T) {
	out, res, _ := transformWith(t, historySource(t), nil)

	bills := rows(t, out, "legacy_bills")
	require.Len(t, bills, 2, "the building's and the analyzer's August invoice (R421)")
	b := bills[0]
	require.Equal(t, "building", b["scope"])
	require.Equal(t, "2026-08", b["period"])
	require.Equal(t, "1000.5", b["total_cost"])
	require.Equal(t, "1234.5", b["total_active_kwh"], "locale-aware numbers")
	require.Equal(t, true, b["reactive_penalty_applied"])
	require.Equal(t, "2026-08-01", b["start_date"])
	require.Equal(t, "/opt/bills/b-2026-08.pdf", b["pdf_path"])
	require.Equal(t, 1000.5, b["payload"].(map[string]any)["totalCost"], "the whole legacy record is kept")
	require.Equal(t, "analyzer", bills[1]["scope"])
	require.Equal(t, legacy.ID("analyzers", "64f0000000000000000000a1").String(), bills[1]["analyzer_id"])
	require.Contains(t, rejectReasons(t, out)["bad_period"], buildingHex+":2026-13")
	require.Equal(t, legacy.Tally{Accepted: 1, Rejected: 1}, res.Summary["building_bill_history"])

	reports := rows(t, out, "legacy_reports")
	require.Len(t, reports, 1)
	require.Equal(t, "/opt/reports/r.xlsx", reports[0]["excel_path"])
	require.Equal(t, map[string]any{"month": "2026-08"}, reports[0]["payload"].(map[string]any)["monthlyReport"])
	require.Contains(t, rejectReasons(t, out)["company_unknown"], "64f000000000000000005a02")

	logs := rows(t, out, "operational_messages")
	require.Len(t, logs, 2)
	require.Equal(t, "job", logs[0]["kind"], "cron → job")
	require.Equal(t, legacy.ID("companies", companyHex).String(), logs[0]["company_id"], "via the analyzer's building")
	require.Equal(t, legacy.ID("analyzers", "64f0000000000000000000a1").String(), logs[0]["related_id"])
	require.Equal(t, "64f000000000000000006a01", logs[0]["metadata"].(map[string]any)["legacy_id"], "load replaces by this (Q-J15)")
	require.Nil(t, logs[1]["related_id"])
	require.Equal(t, "Alarm:zz", logs[1]["metadata"].(map[string]any)["legacy_related"])
	require.Contains(t, rejectReasons(t, out)["outside_retention"], "64f000000000000000006a02", "Q-J10")
}

// TestEveryStepIsIdempotent is R412 over every F14b step: byte-identical output,
// except the two files whose secrets are sealed with random nonces.
func TestEveryStepIsIdempotent(t *testing.T) {
	a, _, _ := transformWith(t, historySource(t), nil)
	b, _, _ := transformWith(t, historySource(t), nil)
	entries, err := os.ReadDir(a)
	require.NoError(t, err)
	for _, e := range entries {
		if e.Name() == "integration_credentials.ndjson" || e.Name() == "smtp_settings.ndjson" {
			continue
		}
		x, err := os.ReadFile(filepath.Join(a, e.Name()))
		require.NoError(t, err)
		y, err := os.ReadFile(filepath.Join(b, e.Name()))
		require.NoError(t, err)
		require.Equal(t, string(x), string(y), e.Name())
	}
}
