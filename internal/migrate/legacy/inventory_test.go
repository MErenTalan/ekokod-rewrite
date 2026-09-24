package legacy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

const (
	companyHex  = "64f000000000000000000c01"
	buildingHex = "64f000000000000000000b01"
)

func fixtureSource(t *testing.T, artifacts string) *legacy.MemSource {
	t.Helper()
	src := legacy.NewMemSource()
	src.Add("companies", bson.D{{Key: "_id", Value: oid(companyHex)}, {Key: "name", Value: "Acme"},
		{Key: "integrations", Value: bson.A{bson.D{{Key: "type", Value: "OSOS"}, {Key: "subType", Value: "Baskent"}, {Key: "password", Value: "enc:00:11"}},
			bson.D{{Key: "type", Value: "ARIL"}, {Key: "subType", Value: "Aril"}}}}})
	src.Add("users",
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d01")}, {Key: "userType", Value: "companyAdmin"}, {Key: "company", Value: oid(companyHex)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d02")}, {Key: "userType", Value: "companyAdmin"}, {Key: "company", Value: oid(companyHex)}},
		bson.D{{Key: "_id", Value: oid("64f000000000000000000d03")}, {Key: "userType", Value: "admin"}, {Key: "company", Value: oid(companyHex)}},
	)
	tariff := func(from string, vat any, ptf bool, kbk any) bson.D {
		return bson.D{{Key: "effectiveFrom", Value: from}, {Key: "usePtfYekdem", Value: ptf}, {Key: "kbk", Value: kbk},
			{Key: "price", Value: bson.D{{Key: "vat_rate", Value: vat}}}}
	}
	src.Add("buildings", bson.D{{Key: "_id", Value: oid(buildingHex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "Merkez"},
		{Key: "tariffs", Value: bson.A{
			tariff("01-01-2025", 20.0, false, nil),
			tariff("2025-07-01", nil, false, nil),
			tariff("2026-01-01T00:00:00.000Z", 20.0, true, bson.D{{Key: "energyKbk", Value: nil}}),
		}},
		{Key: "billHistory", Value: bson.D{{Key: "2026-08", Value: bson.D{{Key: "totalCost", Value: 1000.5}, {Key: "pdfPath", Value: filepath.Join(artifacts, "bills", "b-2026-08.pdf")}}}}}})
	lp := bson.A{ososRow("22/09/2026 10:00:00", "100"), ososRow("23/09/2026 12:00:00", "90")} // a 26 h gap and a drop
	src.Add("analyzers",
		bson.D{{Key: "_id", Value: oid("64f0000000000000000000a1")}, {Key: "building", Value: oid(buildingHex)}, {Key: "subIntegration", Value: "Baskent"},
			{Key: "meterMultiplier", Value: "40"}, {Key: "muhatapNo", Value: "M-1"}, {Key: "energyValues", Value: bson.D{{Key: "loadProfile", Value: lp}}},
			{Key: "billHistory", Value: bson.D{{Key: "2026-08", Value: bson.D{{Key: "totalCost", Value: 250.25}}}}}},
		bson.D{{Key: "_id", Value: oid("64f0000000000000000000a2")}, {Key: "building", Value: oid(buildingHex)}, {Key: "subIntegration", Value: "Aril"},
			{Key: "meterMultiplier", Value: ""}, {Key: "energyValues", Value: bson.D{{Key: "daily", Value: bson.A{ososRow("24/09/2026 00:00:00", "5")}}}}},
	)
	src.Add("carbonfootprint", bson.D{{Key: "_id", Value: oid("64f000000000000000000e01")}})
	return src
}

func TestInventoryIsTheMigrationsSpecification(t *testing.T) {
	artifacts := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(artifacts, "bills"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(artifacts, "bills", "orphan.pdf"), []byte("%PDF"), 0o600))
	inv, err := legacy.TakeInventory(context.Background(), fixtureSource(t, artifacts), now, []string{filepath.Join(artifacts, "bills")})
	require.NoError(t, err)

	require.Equal(t, 3, inv.Collections["users"])
	require.Equal(t, map[string]int{"companyAdmin": 2, "admin": 1}, inv.UsersByRole)
	require.Equal(t, map[string]int{"OSOS": 1, "ARIL": 1}, inv.AnalyzersByProvider)

	osos := inv.Analyzers[0]
	require.Equal(t, "64f0000000000000000000a1", osos.LegacyID)
	require.Equal(t, 2, osos.Readings["load_profile"])
	require.Equal(t, 1, osos.GapsOver24h)
	require.Equal(t, 1, osos.IndexDrops)
	require.False(t, osos.MultiplierDetermined, "OSOS × 40 needs a hand decision")
	require.True(t, inv.Analyzers[1].MultiplierMissing)

	require.Equal(t, 3, inv.Tariffs.Embedded)
	require.Equal(t, map[string]int{"dd-MM-yyyy": 1, "yyyy-MM-dd": 1, "iso8601": 1}, inv.Tariffs.EffectiveFromLayouts)
	require.Equal(t, 1, inv.Tariffs.MissingVAT)
	require.Equal(t, 1, inv.Tariffs.PTFWithoutEnergyKBK)

	require.Len(t, inv.Invoices, 1)
	require.Equal(t, "2026-08", inv.Invoices[0].Period)
	require.Equal(t, 2, inv.Invoices[0].Count)
	require.True(t, decimal.RequireFromString("1250.75").Equal(inv.Invoices[0].Total), "the reconciliation baseline")

	require.Equal(t, 1, inv.Artifacts[0].Files)
	require.Equal(t, []string{filepath.Join(artifacts, "bills", "b-2026-08.pdf")}, inv.MissingOnDisk)
	require.Equal(t, []string{filepath.Join(artifacts, "bills", "orphan.pdf")}, inv.Unreferenced)

	text := inv.Text()
	for _, want := range []string{"users: 3", "OSOS: 1", "gaps > 24 h: 1", "missing VAT: 1", "2026-08", "orphan.pdf"} {
		require.True(t, strings.Contains(text, want), "text report mentions %q:\n%s", want, text)
	}
}
