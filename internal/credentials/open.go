package credentials

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// derefOr returns *p, or fallback if p is nil.
func derefOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

// buildCredentials decrypts cred's secret/extra and assembles an
// integration.Credentials from it and def. It does NOT inject an isolar
// access_token — Open is what layers that on top, and the isolar OAuth
// methods (isolar_oauth.go) build the raw shape directly through this
// function, since AuthorizeURL/ExchangeCode/Refresh must never recurse
// into ISolarAccessToken.
func (s *Service) buildCredentials(ctx context.Context, sc store.Scope, cred model.IntegrationCredential, def model.IntegrationDefinition) (integration.Credentials, error) {
	secretPlain, extraPlain, err := s.deps.Integrations.OpenSecret(ctx, sc, cred.ID)
	if err != nil {
		return integration.Credentials{}, err
	}
	defer zeroBytes(secretPlain)
	defer zeroBytes(extraPlain)

	var endpoints map[string]string
	if len(def.Endpoints) > 0 {
		if err := json.Unmarshal(def.Endpoints, &endpoints); err != nil {
			return integration.Credentials{}, fmt.Errorf("credentials: decode definition endpoints: %w", err)
		}
	}

	var settings integration.Settings
	if len(cred.Settings) > 0 {
		if err := json.Unmarshal(cred.Settings, &settings); err != nil {
			return integration.Credentials{}, fmt.Errorf("credentials: decode settings: %w", err)
		}
	}

	var extraPlainMap map[string]string
	if len(extraPlain) > 0 {
		if err := json.Unmarshal(extraPlain, &extraPlainMap); err != nil {
			return integration.Credentials{}, fmt.Errorf("credentials: decode extra: %w", err)
		}
	}
	extra := make(map[string]integration.Secret, len(extraPlainMap))
	for k, v := range extraPlainMap {
		extra[k] = integration.NewSecret([]byte(v))
	}

	return integration.Credentials{
		CredentialID: cred.ID, CompanyID: cred.CompanyID,
		Provider: integration.Provider(def.Provider), Subtype: def.Subtype,
		Endpoints: endpoints,
		Username:  derefOr(cred.Username, ""),
		Secret:    integration.NewSecret(secretPlain),
		Extra:     extra,
		Settings:  settings,
		BaseURL:   derefOr(cred.Pm5340URL, ""),
		Region:    derefOr(cred.IsolarRegion, ""),

		TokenExpiresAt: cred.TokenExpiresAt,
	}, nil
}

// Open builds the integration.Credentials one adapter call needs: endpoints
// from the credential's Definition, secrets from OpenSecret, verified to
// belong to sc (an id from another company simply is not in
// ListCredentials's result — store.ErrNotFound, never a cross-tenant read).
// For an isolar credential it also fills Extra["access_token"] via
// ISolarAccessToken (refreshing it first if it is expiring within 5 min).
//
// Open satisfies ingest.CredentialOpener structurally — *Service is the
// production implementation Task 10's pipeline calls through.
func (s *Service) Open(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Credentials, error) {
	if !sc.Valid() {
		return integration.Credentials{}, store.ErrInvalidScope
	}
	cred, def, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return integration.Credentials{}, err
	}
	creds, err := s.buildCredentials(ctx, sc, cred, def)
	if err != nil {
		return integration.Credentials{}, err
	}

	if def.Provider == model.IntegrationProviderISolar {
		token, terr := s.isolarAccessToken(ctx, sc, id, cred, def)
		if terr != nil {
			return integration.Credentials{}, terr
		}
		if creds.Extra == nil {
			creds.Extra = make(map[string]integration.Secret, 1)
		}
		creds.Extra["access_token"] = token
	}
	return creds, nil
}
