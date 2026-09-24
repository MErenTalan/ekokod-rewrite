package carbon

import (
	"context"
	"sort"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Selected is the building's declared sub-categories; none when never
// declared (Q-F5).
func (s *Service) Selected(ctx context.Context, sc store.Scope, buildingID uuid.UUID) ([]string, error) {
	// The building decides visibility: an empty list cannot tell "none" from "not yours".
	if _, err := s.d.Buildings.Get(ctx, sc, buildingID); err != nil {
		return nil, err
	}
	rows, err := s.d.Carbon.SelectedActivities(ctx, sc, buildingID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ActivityKey)
	}
	sort.Strings(out)
	return out, nil
}

// SetSelected replaces the declaration (R315).
func (s *Service) SetSelected(ctx context.Context, sc store.Scope, buildingID uuid.UUID, keys []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, k := range keys {
		if _, ok := domain.SubByKey(k); !ok {
			return nil, validation("activity_keys", "unknown")
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	if _, err := s.d.Buildings.Get(ctx, sc, buildingID); err != nil {
		return nil, err
	}
	if err := s.d.Carbon.ReplaceSelectedActivities(ctx, sc, buildingID, out); err != nil {
		return nil, err
	}
	return out, nil
}
