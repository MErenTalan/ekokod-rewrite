package carbon_test

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// fakeCarbon is an in-memory CarbonRepository with the store's scope rules:
// platform factors are readable by every company, company rows only by
// their owner, and building-parented rows only inside the scope.
type fakeCarbon struct {
	store.CarbonRepository
	buildings   map[uuid.UUID]uuid.UUID // building → company
	factors     map[uuid.UUID]model.EmissionFactor
	conversions map[uuid.UUID][]model.EmissionFactorConversion
	selected    map[uuid.UUID][]string
	activities  map[uuid.UUID]model.CarbonActivity
	reports     map[uuid.UUID]model.CarbonReport
}

func newFakeCarbon() *fakeCarbon {
	return &fakeCarbon{buildings: map[uuid.UUID]uuid.UUID{}, factors: map[uuid.UUID]model.EmissionFactor{},
		conversions: map[uuid.UUID][]model.EmissionFactorConversion{}, selected: map[uuid.UUID][]string{},
		activities: map[uuid.UUID]model.CarbonActivity{}, reports: map[uuid.UUID]model.CarbonReport{}}
}

func (f *fakeCarbon) visible(sc store.Scope, company, building uuid.UUID) bool {
	return sc.Valid() && company == sc.CompanyID && f.buildings[building] == company && sc.AllowsBuilding(building)
}

func (f *fakeCarbon) readable(sc store.Scope, fa model.EmissionFactor) bool {
	return fa.CompanyID == nil || *fa.CompanyID == sc.CompanyID
}

func (f *fakeCarbon) Factor(_ context.Context, sc store.Scope, id uuid.UUID) (model.EmissionFactor, error) {
	fa, ok := f.factors[id]
	if !ok || !f.readable(sc, fa) {
		return model.EmissionFactor{}, store.ErrNotFound
	}
	return fa, nil
}

func (f *fakeCarbon) ListFactors(_ context.Context, sc store.Scope, fl store.EmissionFactorFilter) ([]model.EmissionFactor, error) {
	var out []model.EmissionFactor
	for _, fa := range f.factors {
		if fa.CompanyID == nil && !fl.IncludePlatform || fa.CompanyID != nil && *fa.CompanyID != sc.CompanyID {
			continue
		}
		if len(fl.Keys) > 0 && !contains(fl.Keys, fa.Key) || fl.MainCategory != nil && fa.MainCategory != *fl.MainCategory {
			continue
		}
		out = append(out, fa)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	limit, offset := int(fl.Page.Limit), int(fl.Page.Offset)
	if limit <= 0 {
		limit = 100
	}
	if offset >= len(out) {
		return nil, nil
	}
	return out[offset:min(len(out), offset+limit)], nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (f *fakeCarbon) UpsertFactor(_ context.Context, sc store.Scope, fa model.EmissionFactor) (model.EmissionFactor, error) {
	if fa.CompanyID == nil || *fa.CompanyID != sc.CompanyID {
		return model.EmissionFactor{}, store.ErrNotFound
	}
	for id, ex := range f.factors {
		if ex.CompanyID != nil && *ex.CompanyID == sc.CompanyID && ex.Key == fa.Key {
			fa.ID = id
		}
	}
	if fa.ID == uuid.Nil {
		fa.ID = uuid.New()
	}
	f.factors[fa.ID] = fa
	return fa, nil
}

func (f *fakeCarbon) Conversions(_ context.Context, sc store.Scope, id uuid.UUID) ([]model.EmissionFactorConversion, error) {
	fa, ok := f.factors[id]
	if !ok || !f.readable(sc, fa) {
		return nil, store.ErrNotFound
	}
	return f.conversions[id], nil
}

func (f *fakeCarbon) ReplaceConversions(_ context.Context, sc store.Scope, id uuid.UUID, cs []model.EmissionFactorConversion) error {
	fa, ok := f.factors[id]
	if !ok || fa.CompanyID == nil || *fa.CompanyID != sc.CompanyID {
		return store.ErrNotFound
	}
	out := make([]model.EmissionFactorConversion, len(cs))
	for i, c := range cs {
		c.FactorID = id
		out[i] = c
	}
	f.conversions[id] = out
	return nil
}

func (f *fakeCarbon) DeleteCompanyFactors(_ context.Context, sc store.Scope) (int64, error) {
	var n int64
	for id, fa := range f.factors {
		if fa.CompanyID != nil && *fa.CompanyID == sc.CompanyID {
			delete(f.factors, id)
			delete(f.conversions, id)
			n++
			for aid, a := range f.activities {
				if a.FactorID != nil && *a.FactorID == id {
					a.FactorID = nil
					f.activities[aid] = a
				}
			}
		}
	}
	return n, nil
}

func (f *fakeCarbon) SelectedActivities(_ context.Context, sc store.Scope, b uuid.UUID) ([]model.CarbonSelectedActivity, error) {
	if !f.visible(sc, sc.CompanyID, b) {
		return nil, nil
	}
	var out []model.CarbonSelectedActivity
	for _, k := range f.selected[b] {
		out = append(out, model.CarbonSelectedActivity{CompanyID: sc.CompanyID, BuildingID: b, ActivityKey: k})
	}
	return out, nil
}

func (f *fakeCarbon) ReplaceSelectedActivities(_ context.Context, sc store.Scope, b uuid.UUID, keys []string) error {
	if !f.visible(sc, sc.CompanyID, b) {
		return store.ErrNotFound
	}
	f.selected[b] = append([]string(nil), keys...)
	return nil
}

func (f *fakeCarbon) Activity(_ context.Context, sc store.Scope, id uuid.UUID) (model.CarbonActivity, error) {
	a, ok := f.activities[id]
	if !ok || !f.visible(sc, a.CompanyID, a.BuildingID) {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeCarbon) ListActivities(_ context.Context, sc store.Scope, fl store.CarbonActivityFilter) ([]model.CarbonActivity, error) {
	var out []model.CarbonActivity
	for _, a := range f.activities {
		if !f.visible(sc, a.CompanyID, a.BuildingID) {
			continue
		}
		if len(fl.BuildingIDs) > 0 && !containsID(fl.BuildingIDs, a.BuildingID) {
			continue
		}
		if fl.OverlapFrom != nil && a.PeriodEnd.Before(*fl.OverlapFrom) || fl.OverlapTo != nil && a.PeriodStart.After(*fl.OverlapTo) {
			continue
		}
		if fl.ActivityType != nil && a.ActivityType != *fl.ActivityType || fl.IsAutomated != nil && a.IsAutomated != *fl.IsAutomated {
			continue
		}
		if len(fl.Statuses) > 0 && !containsStatus(fl.Statuses, a.Status) || len(fl.Scopes) > 0 && !containsScope(fl.Scopes, a.Scope) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PeriodStart.Equal(out[j].PeriodStart) {
			return out[i].PeriodStart.After(out[j].PeriodStart)
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	limit, offset := int(fl.Page.Limit), int(fl.Page.Offset)
	if limit <= 0 {
		limit = 100
	}
	if offset >= len(out) {
		return nil, nil
	}
	return out[offset:min(len(out), offset+limit)], nil
}

func containsID(list []uuid.UUID, v uuid.UUID) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsStatus(list []model.CarbonStatus, v model.CarbonStatus) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func containsScope(list []model.CarbonScope, v model.CarbonScope) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

var fakeClockBase = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

func (f *fakeCarbon) CreateActivity(_ context.Context, sc store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	if !f.visible(sc, a.CompanyID, a.BuildingID) || a.IsAutomated {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	a.ID = uuid.New()
	a.CreatedAt = fakeClockBase.Add(time.Duration(len(f.activities)) * time.Minute)
	f.activities[a.ID] = a
	return a, nil
}

func (f *fakeCarbon) UpdateActivity(_ context.Context, sc store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	old, ok := f.activities[a.ID]
	if !ok || !f.visible(sc, old.CompanyID, old.BuildingID) {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	a.CreatedAt, a.CompanyID, a.BuildingID = old.CreatedAt, old.CompanyID, old.BuildingID
	f.activities[a.ID] = a
	return a, nil
}

func (f *fakeCarbon) DeleteActivity(_ context.Context, sc store.Scope, id uuid.UUID) error {
	a, ok := f.activities[id]
	if !ok || !f.visible(sc, a.CompanyID, a.BuildingID) {
		return store.ErrNotFound
	}
	delete(f.activities, id)
	return nil
}

func (f *fakeCarbon) SetActivityStatus(_ context.Context, sc store.Scope, id uuid.UUID, st model.CarbonStatus, _ time.Time) (model.CarbonActivity, error) {
	a, ok := f.activities[id]
	if !ok || !f.visible(sc, a.CompanyID, a.BuildingID) {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	a.Status = st
	f.activities[id] = a
	return a, nil
}

func (f *fakeCarbon) UpsertAutomatedActivity(_ context.Context, sc store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	if !f.visible(sc, a.CompanyID, a.BuildingID) {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	a.IsAutomated = true
	for id, ex := range f.activities {
		if ex.IsAutomated && ex.BuildingID == a.BuildingID && ex.ActivityType == a.ActivityType && ex.PeriodStart.Equal(a.PeriodStart) {
			a.ID, a.CreatedAt = id, ex.CreatedAt
			f.activities[id] = a
			return a, nil
		}
	}
	a.ID = uuid.New()
	a.CreatedAt = fakeClockBase.Add(time.Duration(len(f.activities)) * time.Minute)
	f.activities[a.ID] = a
	return a, nil
}

func (f *fakeCarbon) CreateReport(_ context.Context, sc store.Scope, r model.CarbonReport) (model.CarbonReport, error) {
	if !f.visible(sc, r.CompanyID, r.BuildingID) {
		return model.CarbonReport{}, store.ErrNotFound
	}
	r.ID = uuid.New()
	r.CreatedAt = fakeClockBase
	f.reports[r.ID] = r
	return r, nil
}

func (f *fakeCarbon) Report(_ context.Context, sc store.Scope, id uuid.UUID) (model.CarbonReport, error) {
	r, ok := f.reports[id]
	if !ok || !f.visible(sc, r.CompanyID, r.BuildingID) {
		return model.CarbonReport{}, store.ErrNotFound
	}
	return r, nil
}

func (f *fakeCarbon) ListReports(_ context.Context, sc store.Scope, b *uuid.UUID, _ store.Page) ([]model.CarbonReport, error) {
	var out []model.CarbonReport
	for _, r := range f.reports {
		if f.visible(sc, r.CompanyID, r.BuildingID) && (b == nil || *b == r.BuildingID) {
			out = append(out, r)
		}
	}
	return out, nil
}

// fakeBuildings answers Get inside the scope only.
type fakeBuildings struct {
	store.BuildingRepository
	byID map[uuid.UUID]model.Building
}

func (f *fakeBuildings) Get(_ context.Context, sc store.Scope, id uuid.UUID) (model.Building, error) {
	b, ok := f.byID[id]
	if !ok || b.CompanyID != sc.CompanyID || !sc.AllowsBuilding(id) {
		return model.Building{}, store.ErrNotFound
	}
	return b, nil
}

func (f *fakeBuildings) List(_ context.Context, sc store.Scope, _ store.BuildingFilter) ([]model.Building, error) {
	var out []model.Building
	for _, b := range f.byID {
		if b.CompanyID == sc.CompanyID && sc.AllowsBuilding(b.ID) {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// world is one company with two buildings (and one other company), the
// platform grid factor and a natural-gas factor with an m³/kWh conversion.
type world struct {
	carbon             *fakeCarbon
	buildings          *fakeBuildings
	company, other     uuid.UUID
	b1, b2, otherB     uuid.UUID
	gridID, gasID      uuid.UUID
	admin, ba, otherSc store.Scope
}

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptr[T any](v T) *T { return &v }

func newWorld() *world {
	w := &world{carbon: newFakeCarbon(), buildings: &fakeBuildings{byID: map[uuid.UUID]model.Building{}},
		company: uuid.New(), other: uuid.New(), b1: uuid.New(), b2: uuid.New(), otherB: uuid.New()}
	for _, b := range []struct {
		id, company uuid.UUID
		name        string
	}{{w.b1, w.company, "Merkez"}, {w.b2, w.company, "Depo"}, {w.otherB, w.other, "Başka"}} {
		w.carbon.buildings[b.id] = b.company
		w.buildings.byID[b.id] = model.Building{ID: b.id, CompanyID: b.company, Name: b.name, Address: ptr(strings.ToUpper(b.name) + " Cad. 1")}
	}
	w.admin = store.SystemScope(w.company)
	w.ba = store.Scope{CompanyID: w.company, BuildingIDs: []uuid.UUID{w.b1}}
	w.otherSc = store.SystemScope(w.other)
	s2 := model.CarbonScope2
	w.gridID, w.gasID = uuid.New(), uuid.New()
	w.carbon.factors[w.gridID] = model.EmissionFactor{ID: w.gridID, Key: "grid_electricity_tr_2022", Label: "Şebeke",
		MainCategory: "cat_electricity", SubCategories: []string{"sub_grid_electricity"}, CategoryPath: []string{"grid_electricity"},
		BaseFactor: d("0.469"), BaseUnit: "kWh", Scope: &s2, Status: ptr("active"), Source: ptr("TÜ"), SourceYear: ptr(int16(2025))}
	w.carbon.conversions[w.gridID] = []model.EmissionFactorConversion{{FactorID: w.gridID, Unit: "kWh", Multiplier: d("1"), Label: "kWh"}}
	w.carbon.factors[w.gasID] = model.EmissionFactor{ID: w.gasID, Key: "natural_gas", Label: "Doğal gaz", MainCategory: "cat_stationary",
		SubCategories: []string{"sub_space_heating", "sub_process_combustion"}, CategoryPath: []string{"natural_gas"},
		BaseFactor: d("2.06672"), BaseUnit: "m3", Status: ptr("active"), Source: ptr("Defra"), SourceYear: ptr(int16(2025))}
	w.carbon.conversions[w.gasID] = []model.EmissionFactorConversion{
		{FactorID: w.gasID, Unit: "m3", Multiplier: d("1"), Label: "m³"},
		{FactorID: w.gasID, Unit: "kWh", Multiplier: d("0.0948"), Label: "kWh"},
	}
	return w
}

type fakeTenants struct {
	store.AdminTenantRepository
	companies []model.Company
}

func (f *fakeTenants) ListCompanies(_ context.Context, fl store.CompanyFilter) ([]model.Company, error) {
	if int(fl.Page.Offset) >= len(f.companies) {
		return nil, nil
	}
	return f.companies[fl.Page.Offset:], nil
}

type fakeAnalyzers struct {
	store.AnalyzerRepository
	byBuilding map[uuid.UUID][]uuid.UUID
}

func (f *fakeAnalyzers) List(_ context.Context, sc store.Scope, fl store.AnalyzerFilter) ([]model.Analyzer, error) {
	if fl.Page.Offset > 0 || fl.BuildingID == nil || !sc.AllowsBuilding(*fl.BuildingID) {
		return nil, nil
	}
	var out []model.Analyzer
	for _, id := range f.byBuilding[*fl.BuildingID] {
		b := *fl.BuildingID
		out = append(out, model.Analyzer{ID: id, BuildingID: &b})
	}
	return out, nil
}

// fakeAnalytics answers ConsumptionDaily from per-analyzer daily buckets.
type fakeAnalytics struct {
	store.AnalyticsRepository
	buckets []model.ConsumptionBucket
	ranges  []store.TimeRange
}

func (f *fakeAnalytics) ConsumptionDaily(_ context.Context, _ store.Scope, ids []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.ranges = append(f.ranges, r)
	var out []model.ConsumptionBucket
	for _, b := range f.buckets {
		if containsID(ids, b.AnalyzerID) && !b.Bucket.Before(r.From) && b.Bucket.Before(r.To) {
			out = append(out, b)
		}
	}
	return out, nil
}

type fakeOps struct {
	store.OpsRepository
	runs     []model.JobRun
	finished []string
	messages []model.OperationalMessage
}

func (f *fakeOps) StartRun(_ context.Context, _ store.Scope, r model.JobRun) (model.JobRun, error) {
	r.ID = uuid.New()
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeOps) FinishRun(_ context.Context, _ store.Scope, _ uuid.UUID, status string, _, _, _ int32, _ *string, _ []byte, _ time.Time) (model.JobRun, error) {
	f.finished = append(f.finished, status)
	return model.JobRun{}, nil
}

func (f *fakeOps) AppendMessage(_ context.Context, _ store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	f.messages = append(f.messages, m)
	return m, nil
}
