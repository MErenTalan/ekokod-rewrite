// Package integrations administers the provider catalogue (05 §15, R176).
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Errors.
var (
	ErrDefinitionInUse    = perr.New("definition_in_use", 409, "errors.integrations.definitionInUse")
	ErrDefinitionConflict = perr.New("definition_exists", 409, "errors.integrations.definitionExists")
)

// Definitions administers integration definitions.
type Definitions struct {
	Integrations store.IntegrationRepository
	Catalogue    store.AdminCatalogueRepository
}

var endpointKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// validEndpoints checks R176 (as amended): a flat object of snake_case keys to
// non-empty strings; any absolute URL must be https.
func validEndpoints(raw json.RawMessage) error {
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return perr.Validation.WithParams(map[string]any{"endpoints": []string{"object_of_strings"}})
	}
	if len(m) > 64 {
		return perr.Validation.WithParams(map[string]any{"endpoints": []string{"max"}})
	}
	for k, v := range m {
		switch {
		case !endpointKey.MatchString(k):
			return perr.Validation.WithParams(map[string]any{"endpoints." + k: []string{"invalid_key"}})
		case strings.TrimSpace(v) == "" || len(v) > 2048:
			return perr.Validation.WithParams(map[string]any{"endpoints." + k: []string{"required"}})
		case strings.HasPrefix(strings.ToLower(v), "http://"):
			return perr.Validation.WithParams(map[string]any{"endpoints." + k: []string{"https_required"}})
		}
	}
	return nil
}

// List returns the catalogue.
func (d Definitions) List(ctx context.Context, sc store.Scope) ([]model.IntegrationDefinition, error) {
	return d.Integrations.Definitions(ctx, sc)
}

// Create adds a provider/subtype.
func (d Definitions) Create(ctx context.Context, def model.IntegrationDefinition) (model.IntegrationDefinition, error) {
	if err := validEndpoints(def.Endpoints); err != nil {
		return model.IntegrationDefinition{}, err
	}
	created, err := d.Catalogue.CreateIntegrationDefinition(ctx, def)
	if errors.Is(err, store.ErrConflict) {
		return model.IntegrationDefinition{}, ErrDefinitionConflict
	}
	return created, err
}

// Update replaces a definition's subtype and endpoints.
func (d Definitions) Update(ctx context.Context, def model.IntegrationDefinition) (model.IntegrationDefinition, error) {
	if err := validEndpoints(def.Endpoints); err != nil {
		return model.IntegrationDefinition{}, err
	}
	updated, err := d.Catalogue.UpdateIntegrationDefinition(ctx, def)
	if errors.Is(err, store.ErrConflict) {
		return model.IntegrationDefinition{}, ErrDefinitionConflict
	}
	return updated, err
}

// Delete removes an unreferenced definition.
func (d Definitions) Delete(ctx context.Context, id uuid.UUID) error {
	err := d.Catalogue.DeleteIntegrationDefinition(ctx, id)
	if errors.Is(err, store.ErrConflict) {
		return ErrDefinitionInUse
	}
	return err
}
