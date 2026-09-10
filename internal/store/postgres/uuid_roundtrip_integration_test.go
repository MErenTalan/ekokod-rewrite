//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// sqlc.yaml maps the SQL `uuid` type to github.com/google/uuid.UUID rather than
// pgtype.UUID, so that store.Scope's CompanyID/BuildingIDs and the generated
// query parameters are the same type and no repository in F1 carries conversion
// boilerplate. That mapping is a claim about pgx/v5's codec, not about sqlc:
// sqlc will happily emit `uuid.UUID` in a struct field whether or not pgx can
// actually put a value there. A compile is therefore no evidence at all, and
// the failure mode of being wrong is a scan error in every repository at once.
//
// These tests are the evidence. They run against a real TimescaleDB container
// and exercise the generated code path in both directions:
//
//   - encode: a uuid.UUID passed as a query parameter, and a []uuid.UUID passed
//     as a `= any($3::uuid[])` array parameter (the store.Scope shape every
//     scoped list query in Tasks 9-11 copies);
//   - decode: a NOT NULL uuid primary key into uuid.UUID, and a NULLABLE uuid
//     foreign key into *uuid.UUID.
//
// The nullable case is the one with a real bug behind it. If a SQL NULL decoded
// into a non-nil *uuid.UUID holding uuid.Nil, a null foreign key would be
// indistinguishable from a present-but-invalid id -- and uuid.Nil is precisely
// what store.Scope treats as invalid. So it is asserted explicitly, both
// directions, rather than left implied by "the round trip worked".
//
// buildings is used rather than analyzers because it has exactly the shape that
// matters -- `id uuid primary key` NOT NULL and `responsible_user_id uuid`
// NULLABLE -- and, unlike analyzers, is reachable through generated queries, so
// the test covers the code Tasks 9-11 will actually run. analyzers is covered
// too, one level down, to show the mapping is not special to one table.

// TestGeneratedUUIDRoundTripsThroughPostgres covers the not-null path end to
// end through sqlc-generated code.
func TestGeneratedUUIDRoundTripsThroughPostgres(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool := testfixtures.NewPool(t, dsn)
	db := postgres.New(pool)
	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)

	// Both parameters are google/uuid values, encoded by pgx with no codec
	// registration on the pool.
	got, err := db.GetBuilding(ctx, sqlcgen.GetBuildingParams{ID: buildingID, CompanyID: companyID})
	require.NoError(t, err)

	require.Equal(t, buildingID, got.ID, "not-null uuid primary key did not round-trip")
	require.Equal(t, companyID, got.CompanyID, "not-null uuid foreign key did not round-trip")
	require.NotEqual(t, uuid.Nil, got.ID, "a real id must never decode as uuid.Nil")
}

// TestGeneratedNullableUUIDDecodesAsNilNotUUIDNil is the specific guard against
// a SQL NULL arriving as uuid.Nil.
func TestGeneratedNullableUUIDDecodesAsNilNotUUIDNil(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool := testfixtures.NewPool(t, dsn)
	db := postgres.New(pool)
	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)

	// seedCompanyAndBuilding writes only the not-null columns, so this
	// building's responsible_user_id is SQL NULL.
	got, err := db.GetBuilding(ctx, sqlcgen.GetBuildingParams{ID: buildingID, CompanyID: companyID})
	require.NoError(t, err)
	require.Nil(t, got.ResponsibleUserID,
		"a NULL uuid must decode as a nil *uuid.UUID: a non-nil pointer to "+
			"uuid.Nil would make a null foreign key indistinguishable from a "+
			"present-but-invalid id, which store.Scope treats as invalid")

	// And the populated case, so the assertion above is not passing merely
	// because the column never decodes to anything.
	var userID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into users (company_id, name, email, password_hash, role)
		 values ($1, 'Seed User', $2, 'x', 'admin') returning id`,
		companyID, "uuid-roundtrip-"+uuid.NewString()+"@example.test",
	).Scan(&userID))

	_, err = pool.Exec(ctx,
		`update buildings set responsible_user_id = $1 where id = $2`, userID, buildingID)
	require.NoError(t, err)

	got, err = db.GetBuilding(ctx, sqlcgen.GetBuildingParams{ID: buildingID, CompanyID: companyID})
	require.NoError(t, err)
	require.NotNil(t, got.ResponsibleUserID, "a populated nullable uuid decoded as nil")
	require.Equal(t, userID, *got.ResponsibleUserID,
		"populated nullable uuid did not round-trip")
	require.NotEqual(t, uuid.Nil, *got.ResponsibleUserID)
}

// TestGeneratedUUIDArrayParameterRoundTrips exercises the `= any($3::uuid[])`
// half of the store.Scope shape: a []uuid.UUID encoded as a uuid[] parameter.
// This is the pattern every scoped list query in Tasks 9-11 copies, and it is a
// separate pgx code path from a scalar uuid parameter.
func TestGeneratedUUIDArrayParameterRoundTrips(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool := testfixtures.NewPool(t, dsn)
	db := postgres.New(pool)
	companyID, wantedID := seedCompanyAndBuilding(t, ctx, pool)

	var otherID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into buildings (company_id, name) values ($1, $2) returning id`,
		companyID, "out of scope",
	).Scan(&otherID))

	// AllBuildings false: only the ids in the array come back.
	scoped, err := db.ListBuildingsForScope(ctx, sqlcgen.ListBuildingsForScopeParams{CompanyID: companyID, AllBuildings: false, BuildingIds: []uuid.UUID{wantedID}})
	require.NoError(t, err)
	require.Len(t, scoped, 1, "uuid[] parameter did not filter as expected")
	require.Equal(t, wantedID, scoped[0].ID)

	// An empty slice must select nothing rather than erroring or matching all.
	empty, err := db.ListBuildingsForScope(ctx, sqlcgen.ListBuildingsForScopeParams{CompanyID: companyID, AllBuildings: false, BuildingIds: nil})
	require.NoError(t, err)
	require.Empty(t, empty, "an empty building_ids array must match no rows")

	// AllBuildings true: the array is ignored and every building comes back.
	all, err := db.ListBuildingsForScope(ctx, sqlcgen.ListBuildingsForScopeParams{CompanyID: companyID, AllBuildings: true, BuildingIds: nil})
	require.NoError(t, err)
	require.Len(t, all, 2)
}

// TestNullableUUIDForeignKeyOnAnalyzers repeats the NULL check one level down,
// on the table the plan named, to show the mapping is not special to buildings.
// analyzers has no generated query yet, so this reads the column directly into
// the same *uuid.UUID shape sqlc generates for Analyzer.BuildingID.
func TestNullableUUIDForeignKeyOnAnalyzers(t *testing.T) {
	ctx := context.Background()
	dsn := testfixtures.StartPostgresUnmigrated(t)
	require.NoError(t, postgres.MigrateUp(ctx, dsn, testfixtures.DiscardLogger()))

	pool := testfixtures.NewPool(t, dsn)
	companyID, buildingID := seedCompanyAndBuilding(t, ctx, pool)

	insert := `insert into analyzers (company_id, building_id, provider, provider_subtype, installation_number)
	           values ($1, $2, 'osos', 'Baskent', $3) returning id, building_id`

	var unlinkedID uuid.UUID
	var unlinkedBuilding *uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, insert, companyID, nil, "no-building").
		Scan(&unlinkedID, &unlinkedBuilding))
	require.NotEqual(t, uuid.Nil, unlinkedID)
	require.Nil(t, unlinkedBuilding, "a NULL building_id must decode as nil, not uuid.Nil")

	var linkedID uuid.UUID
	var linkedBuilding *uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, insert, companyID, buildingID, "with-building").
		Scan(&linkedID, &linkedBuilding))
	require.NotNil(t, linkedBuilding)
	require.Equal(t, buildingID, *linkedBuilding)
}
