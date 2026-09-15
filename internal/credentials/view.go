package credentials

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// View is the ONLY read model of a credential (05 §15 "never returns
// secrets"). It has no field that can hold secret material: HasSecret is a
// bool (a bool cannot itself carry a secret's bytes), and ExtraKeys holds
// only the NAMES of the Extra map's keys — never their values.
// TestViewCarriesNoSecretMaterial pins this at the type level: a future
// field of type integration.Secret or []byte fails it immediately, and a
// future bool must be named exactly "HasSecret" or the test fails on the
// name check, so a reviewer sees every bool this type ever grows.
type View struct {
	ID, DefinitionID uuid.UUID
	Provider         model.IntegrationProvider
	Subtype          string
	Username         *string
	HasSecret        bool
	// ExtraKeys is the sorted list of Extra's keys, e.g.
	// ["app_id","app_key","secret_key"] — names only, never values.
	ExtraKeys    []string
	Settings     json.RawMessage
	PM5340URL    *string
	IsolarRegion *string
	IsActive     bool

	TokenExpiresAt, LastVerifiedAt *time.Time
	UpdatedAt                      time.Time
}

// viewOf builds a View for one credential row, given its already-resolved
// definition. It calls OpenSecret ONLY to read Extra's key NAMES (never a
// value, and never returning one): ExtraEnc is one sealed JSON blob for the
// whole map, so there is no way to learn which keys it holds without
// opening it. Every decrypted value is discarded before this function
// returns; nothing it reads ever reaches the View it builds.
func (s *Service) viewOf(ctx context.Context, sc store.Scope, c model.IntegrationCredential, def model.IntegrationDefinition) (View, error) {
	var extraKeys []string
	if len(c.ExtraEnc) > 0 {
		_, extraPlain, err := s.deps.Integrations.OpenSecret(ctx, sc, c.ID)
		if err != nil {
			return View{}, err
		}
		var m map[string]json.RawMessage
		if len(extraPlain) > 0 {
			if err := json.Unmarshal(extraPlain, &m); err != nil {
				zeroBytes(extraPlain)
				return View{}, err
			}
		}
		zeroBytes(extraPlain)
		extraKeys = make([]string, 0, len(m))
		for k, v := range m {
			extraKeys = append(extraKeys, k)
			zeroBytes(v) // best effort: v's bytes still hold the sealed value's plaintext JSON encoding
		}
		sort.Strings(extraKeys)
	}

	return View{
		ID: c.ID, DefinitionID: c.DefinitionID,
		Provider: def.Provider, Subtype: def.Subtype,
		Username:     c.Username,
		HasSecret:    len(c.SecretEnc) > 0,
		ExtraKeys:    extraKeys,
		Settings:     c.Settings,
		PM5340URL:    c.Pm5340URL,
		IsolarRegion: c.IsolarRegion,
		IsActive:     c.IsActive,

		TokenExpiresAt: c.TokenExpiresAt,
		LastVerifiedAt: c.LastVerifiedAt,
		UpdatedAt:      c.UpdatedAt,
	}, nil
}
