package legacy_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
)

const (
	company2Hex  = "64f000000000000000000c02"
	building3Hex = "64f000000000000000000b03"
	user9Hex     = "64f000000000000000000d09"
)

// crossTenantSource puts a user of company 2 in charge of a company-1 building.
func crossTenantSource(t *testing.T) *legacy.MemSource {
	src := carbonISOSource(t)
	src.Add("users", bson.D{{Key: "_id", Value: oid(user9Hex)}, {Key: "name", Value: "Q"}, {Key: "email", Value: "q@beta.test"}, {Key: "password", Value: bcryptHash},
		{Key: "userType", Value: "company-admin"}, {Key: "company", Value: oid(company2Hex)}})
	src.Add("buildings", bson.D{{Key: "_id", Value: oid(building3Hex)}, {Key: "company_id", Value: oid(companyHex)}, {Key: "name", Value: "Ek"},
		{Key: "user_in_charge", Value: oid(user9Hex)}})
	// An iSolar-linked plant points at its company's credential (R424): load order must allow it.
	src.Add("integrations", bson.D{{Key: "_id", Value: oid("64f000000000000000000f03")}, {Key: "type", Value: "ISOLAR"}, {Key: "subType", Value: "iSolarCloud"}})
	src.Replace("companies", bson.D{{Key: "_id", Value: oid(companyHex)}, {Key: "name", Value: "Acme"},
		{Key: "integrations", Value: bson.A{
			bson.D{{Key: "type", Value: "OSOS"}, {Key: "subType", Value: "Baskent"}, {Key: "username", Value: "acme"}, {Key: "password", Value: vecPrimary}},
			bson.D{{Key: "type", Value: "ARIL"}, {Key: "subType", Value: "Aril"}, {Key: "username", Value: "acme-aril"}},
			bson.D{{Key: "type", Value: "ISOLAR"}, {Key: "subType", Value: "iSolarCloud"}, {Key: "isolar_appkey", Value: "k"}, {Key: "isolar_region", Value: "EU"}},
		}}},
		bson.D{{Key: "_id", Value: oid(company2Hex)}, {Key: "name", Value: "Beta"}})
	src.Add("notes", bson.D{{Key: "_id", Value: oid("64f00000000000000000ab09")}, {Key: "userId", Value: user9Hex}, {Key: "subItem", Value: "5.1"}, {Key: "text", Value: "Beta"}})
	return src
}

func TestNoRowLinksTwoCompanies(t *testing.T) {
	out, _, _ := transformWith(t, crossTenantSource(t), nil)
	b := byID(rows(t, out, "buildings"), legacy.ID("buildings", building3Hex).String())
	require.NotNil(t, b)
	require.Nil(t, b["responsible_user_id"], "a user of another company is never a building's responsible user")
	require.Contains(t, rejectReasons(t, out)["building_ambiguous"], "64f00000000000000000ab09",
		"company 2 has no building: its user's ISO work is never moved into company 1's building")
}
