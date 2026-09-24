package auth_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// The web navigation tests read this fixture; it must be exactly PermissionsFor, so the two cannot drift.
func TestPermissionsFixtureMatchesTable(t *testing.T) {
	raw, err := os.ReadFile("../../web/src/lib/session/permissions.fixture.json")
	require.NoError(t, err)
	var fixture map[string][]string
	require.NoError(t, json.Unmarshal(raw, &fixture))
	want := map[string][]string{}
	for _, role := range model.UserRoles() {
		want[string(role)] = auth.PermissionsFor(role)
	}
	require.Equal(t, want, fixture, "regenerate web/src/lib/session/permissions.fixture.json from auth.PermissionsFor")
}
