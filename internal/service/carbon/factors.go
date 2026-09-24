package carbon

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// FactorView is one effective catalogue entry (R302).
type FactorView struct {
	model.EmissionFactor
	Conversions        []model.EmissionFactorConversion
	Overridden         bool
	PlatformBaseFactor *decimal.Decimal
}

// Usable reports whether new entries may use the factor (R302).
func (f FactorView) Usable() bool { return f.Status == nil || *f.Status == "active" }

// FactorQuery narrows the catalogue.
type FactorQuery struct{ SubCategory, MainCategory, Q string }

// FactorOverride is R303's body.
type FactorOverride struct {
	BaseFactor decimal.Decimal
	Source     *string
	SourceYear *int16
	SourceURL  *string
}

var maxFactor = decimal.NewFromInt(1_000_000)

func (s *Service) listFactors(ctx context.Context, sc store.Scope, f store.EmissionFactorFilter) ([]model.EmissionFactor, error) {
	var out []model.EmissionFactor
	f.IncludePlatform = true
	for offset := int32(0); ; offset += pageSize {
		f.Page = store.Page{Limit: pageSize, Offset: offset}
		page, err := s.d.Carbon.ListFactors(ctx, sc, f)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}

// effective merges platform and company rows: a company row shadows the
// platform row with the same key (R302).
func effective(rows []model.EmissionFactor) []FactorView {
	byKey := map[string]*FactorView{}
	for _, r := range rows {
		v, ok := byKey[r.Key]
		if !ok {
			v = &FactorView{}
			byKey[r.Key] = v
		}
		if r.CompanyID == nil {
			base := r.BaseFactor
			v.PlatformBaseFactor = &base
			if !v.Overridden {
				v.EmissionFactor = r
			}
			continue
		}
		v.EmissionFactor, v.Overridden = r, true
	}
	out := make([]FactorView, 0, len(byKey))
	for _, v := range byKey {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (s *Service) withConversions(ctx context.Context, sc store.Scope, v *FactorView) error {
	cs, err := s.d.Carbon.Conversions(ctx, sc, v.ID)
	if err != nil {
		return err
	}
	v.Conversions = nil
	for _, c := range cs {
		if c.Unit != "" { // the left join yields one empty row for a factor without conversions
			v.Conversions = append(v.Conversions, c)
		}
	}
	return nil
}

func matches(v FactorView, q FactorQuery) bool {
	if q.SubCategory != "" && !contains(v.SubCategories, q.SubCategory) {
		return false
	}
	if q.Q != "" {
		needle := strings.ToLower(strings.TrimSpace(q.Q))
		return strings.Contains(strings.ToLower(v.Key), needle) || strings.Contains(strings.ToLower(v.Label), needle)
	}
	return true
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Factors is the company's effective catalogue with conversions (R302).
func (s *Service) Factors(ctx context.Context, sc store.Scope, q FactorQuery) ([]FactorView, error) {
	f := store.EmissionFactorFilter{}
	if q.MainCategory != "" {
		f.MainCategory = &q.MainCategory
	}
	rows, err := s.listFactors(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	var out []FactorView
	for _, v := range effective(rows) {
		if !matches(v, q) {
			continue
		}
		if err := s.withConversions(ctx, sc, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Effective resolves one key in the company's catalogue, else ErrNotFound.
func (s *Service) Effective(ctx context.Context, sc store.Scope, key string) (FactorView, error) {
	rows, err := s.listFactors(ctx, sc, store.EmissionFactorFilter{Keys: []string{key}})
	if err != nil {
		return FactorView{}, err
	}
	views := effective(rows)
	if len(views) == 0 {
		return FactorView{}, store.ErrNotFound
	}
	v := views[0]
	return v, s.withConversions(ctx, sc, &v)
}

// GridFactor is R262/R313: the company override first, else the platform
// row; nil when neither exists.
func (s *Service) GridFactor(ctx context.Context, sc store.Scope) (*FactorView, error) {
	v, err := s.Effective(ctx, sc, domain.GridFactorKey)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// OverrideFactor is R303: a platform id gets a company shadow (descriptive
// fields and conversions copied), an own id is updated in place.
func (s *Service) OverrideFactor(ctx context.Context, sc store.Scope, id uuid.UUID, in FactorOverride) (FactorView, error) {
	if !in.BaseFactor.IsPositive() || in.BaseFactor.GreaterThan(maxFactor) {
		return FactorView{}, validation("base_factor", "range")
	}
	if in.SourceYear != nil && (*in.SourceYear < 1990 || *in.SourceYear > 2100) {
		return FactorView{}, validation("source_year", "range")
	}
	if in.SourceURL != nil && len(*in.SourceURL) > 500 || in.Source != nil && len(*in.Source) > 200 {
		return FactorView{}, validation("source", "too_long")
	}
	f, err := s.d.Carbon.Factor(ctx, sc, id)
	if err != nil {
		return FactorView{}, err
	}
	platform := f.CompanyID == nil
	next := f
	if platform {
		companyID := sc.CompanyID
		next.ID, next.CompanyID = uuid.Nil, &companyID
	}
	next.BaseFactor = in.BaseFactor
	if in.Source != nil {
		next.Source = in.Source
	}
	if in.SourceYear != nil {
		next.SourceYear = in.SourceYear
	}
	if in.SourceURL != nil {
		next.SourceURL = in.SourceURL
	}
	saved, err := s.d.Carbon.UpsertFactor(ctx, sc, next)
	if err != nil {
		return FactorView{}, err
	}
	if platform {
		cs, err := s.d.Carbon.Conversions(ctx, sc, f.ID)
		if err != nil {
			return FactorView{}, err
		}
		var copied []model.EmissionFactorConversion
		for _, c := range cs {
			if c.Unit != "" {
				c.FactorID = saved.ID
				copied = append(copied, c)
			}
		}
		if err := s.d.Carbon.ReplaceConversions(ctx, sc, saved.ID, copied); err != nil {
			return FactorView{}, err
		}
	}
	return s.Effective(ctx, sc, saved.Key)
}

// ResetFactors is R304: every company override is removed.
func (s *Service) ResetFactors(ctx context.Context, sc store.Scope) error {
	_, err := s.d.Carbon.DeleteCompanyFactors(ctx, sc)
	return err
}
