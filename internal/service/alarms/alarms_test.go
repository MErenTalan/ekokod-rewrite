package alarms_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var (
	companyID  = uuid.MustParse("00000000-0000-0000-0000-0000000000c0")
	buildingA  = uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	buildingB  = uuid.MustParse("00000000-0000-0000-0000-0000000000b2")
	analyzerA  = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	analyzerB  = uuid.MustParse("00000000-0000-0000-0000-0000000000a2")
	foreignAna = uuid.MustParse("00000000-0000-0000-0000-0000000000a9")
)

// companyScope spans the whole company; buildingScope sees only buildingA,
// which is what R213's intersection is judged on.
var (
	companyScope  = store.Scope{CompanyID: companyID, AllBuildings: true}
	buildingScope = store.Scope{CompanyID: companyID, BuildingIDs: []uuid.UUID{buildingA}}
)

func dec(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
func i32(v int32) *int32            { return &v }

func unit(u model.PeriodUnit) *model.PeriodUnit { return &u }

// fakeAlarms is an in-memory AlarmRepository: only the methods the service
// calls are implemented, the rest panic through the embedded nil interface.
type fakeAlarms struct {
	store.AlarmRepository
	stored    map[uuid.UUID]model.Alarm
	analyzers map[uuid.UUID][]uuid.UUID
	channels  map[uuid.UUID][]model.AlarmChannel
	// visibleAnalyzers is the set the analyzer repository will admit; anything
	// else makes ReplaceAnalyzers answer ErrNotFound, as the real one does.
	visible map[uuid.UUID]bool
}

func newFakeAlarms() *fakeAlarms {
	return &fakeAlarms{stored: map[uuid.UUID]model.Alarm{}, analyzers: map[uuid.UUID][]uuid.UUID{},
		channels: map[uuid.UUID][]model.AlarmChannel{}, visible: map[uuid.UUID]bool{}}
}

func (f *fakeAlarms) Get(_ context.Context, s store.Scope, id uuid.UUID) (model.Alarm, error) {
	a, ok := f.stored[id]
	if !ok || a.CompanyID != s.CompanyID {
		return model.Alarm{}, store.ErrNotFound
	}
	return a, nil
}

func (f *fakeAlarms) List(_ context.Context, s store.Scope, fl store.AlarmFilter) ([]model.Alarm, error) {
	var out []model.Alarm
	for _, a := range f.stored {
		if a.CompanyID != s.CompanyID {
			continue
		}
		if fl.IsEnabled != nil && a.IsEnabled != *fl.IsEnabled {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeAlarms) Create(_ context.Context, s store.Scope, a model.Alarm) (model.Alarm, error) {
	a.ID, a.CompanyID = uuid.New(), s.CompanyID
	f.stored[a.ID] = a
	return a, nil
}

func (f *fakeAlarms) Update(_ context.Context, s store.Scope, a model.Alarm) (model.Alarm, error) {
	cur, ok := f.stored[a.ID]
	if !ok || cur.CompanyID != s.CompanyID {
		return model.Alarm{}, store.ErrNotFound
	}
	a.CompanyID = cur.CompanyID
	f.stored[a.ID] = a
	return a, nil
}

func (f *fakeAlarms) SoftDelete(_ context.Context, s store.Scope, id uuid.UUID, _ time.Time) error {
	if a, ok := f.stored[id]; !ok || a.CompanyID != s.CompanyID {
		return store.ErrNotFound
	}
	delete(f.stored, id)
	return nil
}

func (f *fakeAlarms) Analyzers(_ context.Context, s store.Scope, alarmID uuid.UUID) ([]model.AlarmAnalyzer, error) {
	if a, ok := f.stored[alarmID]; !ok || a.CompanyID != s.CompanyID {
		return nil, store.ErrNotFound
	}
	var out []model.AlarmAnalyzer
	for _, id := range f.analyzers[alarmID] {
		out = append(out, model.AlarmAnalyzer{AlarmID: alarmID, AnalyzerID: id})
	}
	return out, nil
}

func (f *fakeAlarms) ReplaceAnalyzers(_ context.Context, s store.Scope, alarmID uuid.UUID, ids []uuid.UUID) error {
	if a, ok := f.stored[alarmID]; !ok || a.CompanyID != s.CompanyID {
		return store.ErrNotFound
	}
	// The real repository refuses the WHOLE call when any id is outside the
	// scope, and writes nothing.
	for _, id := range ids {
		if !f.visible[id] || !scopeAllows(s, id) {
			return store.ErrNotFound
		}
	}
	f.analyzers[alarmID] = ids
	return nil
}

func (f *fakeAlarms) Channels(_ context.Context, s store.Scope, alarmID uuid.UUID) ([]model.AlarmChannel, error) {
	if a, ok := f.stored[alarmID]; !ok || a.CompanyID != s.CompanyID {
		return nil, store.ErrNotFound
	}
	return f.channels[alarmID], nil
}

func (f *fakeAlarms) ReplaceChannels(_ context.Context, s store.Scope, alarmID uuid.UUID, cs []model.AlarmChannel) error {
	if a, ok := f.stored[alarmID]; !ok || a.CompanyID != s.CompanyID {
		return store.ErrNotFound
	}
	f.channels[alarmID] = cs
	return nil
}

func (f *fakeAlarms) ListEvents(_ context.Context, s store.Scope, fl store.AlarmEventFilter) ([]model.AlarmEvent, error) {
	if fl.AlarmID != nil {
		if a, ok := f.stored[*fl.AlarmID]; !ok || a.CompanyID != s.CompanyID {
			return nil, store.ErrNotFound
		}
	}
	return nil, nil
}

// analyzerBuilding is the fixture's meter-to-building map.
var analyzerBuilding = map[uuid.UUID]uuid.UUID{
	analyzerA: buildingA, analyzerB: buildingB, foreignAna: buildingB,
}

func scopeAllows(s store.Scope, analyzerID uuid.UUID) bool {
	if s.AllBuildings {
		return true
	}
	b, ok := analyzerBuilding[analyzerID]
	if !ok {
		return false
	}
	for _, allowed := range s.BuildingIDs {
		if allowed == b {
			return true
		}
	}
	return false
}

type fakeAnalyzers struct {
	store.AnalyzerRepository
	known map[uuid.UUID]bool
}

// List applies the Scope's building branch, exactly as the real repository
// does — which is what makes the service's intersection real.
func (f fakeAnalyzers) List(_ context.Context, s store.Scope, fl store.AnalyzerFilter) ([]model.Analyzer, error) {
	var out []model.Analyzer
	for _, id := range fl.IDs {
		if !f.known[id] || id == foreignAna || !scopeAllows(s, id) {
			continue
		}
		b := analyzerBuilding[id]
		last := time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC)
		out = append(out, model.Analyzer{ID: id, CompanyID: s.CompanyID, BuildingID: &b,
			InstallationNumber: "INST-" + id.String()[:4], LastReadingAt: &last})
	}
	return out, nil
}

func newTestService(t *testing.T) (*alarms.Service, *fakeAlarms) {
	t.Helper()
	repo := newFakeAlarms()
	repo.visible[analyzerA], repo.visible[analyzerB] = true, true
	svc, err := alarms.New(alarms.Deps{
		Alarms:    repo,
		Analyzers: fakeAnalyzers{known: map[uuid.UUID]bool{analyzerA: true, analyzerB: true}},
		Clock:     clock.NewFake(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	return svc, repo
}

func commsInput(name string, analyzerIDs ...uuid.UUID) alarms.Input {
	return alarms.Input{Name: name, Type: model.AlarmTypeDataCommunication, IsEnabled: true,
		Alarm: model.Alarm{CommunicationThresholdHours: i32(6)}, AnalyzerIDs: analyzerIDs}
}

func mustErr[T any](_ T, err error) error { return err }

func TestCreateRejectsInvalidSettings(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), companyScope, alarms.Input{
		Name: "Gerilim", Type: model.AlarmTypeCurrentVoltagePower,
		Alarm:       model.Alarm{VoltageMax: dec("400")},
		AnalyzerIDs: []uuid.UUID{analyzerA},
	})
	// R212 travels from the domain package to a 422 with per-field codes.
	require.Equal(t, http.StatusUnprocessableEntity, perr.StatusOf(err))
	require.Equal(t, "validation_failed", perr.CodeOf(err))
}

func TestCreateClearsSettingsOfOtherTypes(t *testing.T) {
	t.Parallel()
	svc, repo := newTestService(t)
	// A reactive threshold on a comms rule is a leftover from the dialog's type
	// switch; it must never reach the row (model.Alarm's own warning).
	in := commsInput("İletişim", analyzerA)
	in.Alarm.InductiveRatioThreshold = dec("20")
	got, err := svc.Create(context.Background(), companyScope, in)
	require.NoError(t, err)
	require.Nil(t, got.Alarm.InductiveRatioThreshold)
	require.Nil(t, repo.stored[got.Alarm.ID].InductiveRatioThreshold)
	require.NotNil(t, repo.stored[got.Alarm.ID].CommunicationThresholdHours)
}

func TestSettingsForNeverCopiesVoltage(t *testing.T) {
	t.Parallel()
	// R212's SECOND line of defence, tested directly: no route through Create
	// can reach settingsFor with a voltage threshold on a power rule, because
	// alarm.Validate rejects that first. Exercising it only through Create
	// would leave this branch unguarded the day validation is loosened.
	row := alarms.SettingsFor(alarms.Input{
		Name: "Güç", Type: model.AlarmTypeCurrentVoltagePower, IsEnabled: true,
		Alarm: model.Alarm{PowerMax: dec("100"), VoltageMax: dec("400"), VoltageMin: dec("180")},
	})
	require.Nil(t, row.VoltageMax)
	require.Nil(t, row.VoltageMin)
	require.Equal(t, "100", row.PowerMax.String())
}

func TestSettingsForDropsEveryOtherTypesFields(t *testing.T) {
	t.Parallel()
	// One case per type, so a field added to the wrong branch is caught.
	full := model.Alarm{
		InductiveRatioThreshold: dec("20"), CommunicationThresholdHours: i32(6),
		PowerMax: dec("100"), InvoiceThresholdPct: dec("20"),
	}
	comms := alarms.SettingsFor(alarms.Input{Type: model.AlarmTypeDataCommunication, Alarm: full})
	require.NotNil(t, comms.CommunicationThresholdHours)
	require.Nil(t, comms.InductiveRatioThreshold)
	require.Nil(t, comms.PowerMax)
	require.Nil(t, comms.InvoiceThresholdPct)

	invoice := alarms.SettingsFor(alarms.Input{Type: model.AlarmTypeInvoiceIncrease, Alarm: full})
	require.NotNil(t, invoice.InvoiceThresholdPct)
	require.Nil(t, invoice.CommunicationThresholdHours)
}

func TestCreateRequiresEveryAnalyzerInScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), buildingScope, commsInput("Karışık", analyzerA, analyzerB))
	// R213: one indistinguishable 404, never a 403 that confirms existence.
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCreateFailsWhenReplaceAnalyzersIsRefused(t *testing.T) {
	t.Parallel()
	// A company-wide scope, so R213's intersection does NOT fire and cannot
	// mask the result: the only thing that can refuse this create is
	// ReplaceAnalyzers' own error being returned rather than swallowed.
	svc, repo := newTestService(t)
	repo.visible[analyzerB] = false

	_, err := svc.Create(context.Background(), companyScope, commsInput("Görünmez", analyzerB))
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestListShowsARuleWhenOneAnalyzerIsInScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.Create(ctx, companyScope, commsInput("Bizim bina", analyzerA))
	require.NoError(t, err)
	_, err = svc.Create(ctx, companyScope, commsInput("Başka bina", analyzerB))
	require.NoError(t, err)

	// R213, legacy semantics: a building admin sees a company-wide rule that
	// touches one of their meters, and not one that touches none of them.
	got, err := svc.List(ctx, buildingScope, store.AlarmFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "Bizim bina", got[0].Alarm.Name)

	all, err := svc.List(ctx, companyScope, store.AlarmFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestGetHidesARuleWithNoAnalyzerInScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	ctx := context.Background()
	other, err := svc.Create(ctx, companyScope, commsInput("Başka bina", analyzerB))
	require.NoError(t, err)

	_, err = svc.Get(ctx, buildingScope, other.Alarm.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	// The company-wide scope still sees it, so the 404 is about scope, not existence.
	_, err = svc.Get(ctx, companyScope, other.Alarm.ID)
	require.NoError(t, err)
}

func TestRuleWithNoAnalyzersIsCompanyWideOnly(t *testing.T) {
	t.Parallel()
	// A rule whose analyzers were all deleted cannot be intersected, so only a
	// scope that spans the company may see it (R213).
	svc, repo := newTestService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, companyScope, commsInput("Yetim", analyzerA))
	require.NoError(t, err)
	repo.analyzers[created.Alarm.ID] = nil

	_, err = svc.Get(ctx, buildingScope, created.Alarm.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = svc.Get(ctx, companyScope, created.Alarm.ID)
	require.NoError(t, err)
}

func TestChannelsAreValidatedDeduplicatedAndCapped(t *testing.T) {
	t.Parallel()
	svc, repo := newTestService(t)
	ctx := context.Background()

	bad := commsInput("Kanal", analyzerA)
	bad.Channels = []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "not-an-email"}}
	require.Equal(t, http.StatusUnprocessableEntity,
		perr.StatusOf(mustErr(svc.Create(ctx, companyScope, bad))))

	badSMS := commsInput("Kanal", analyzerA)
	badSMS.Channels = []model.AlarmChannel{{Channel: model.NotifyChannelSMS, Target: "0555 123 45 67"}}
	require.Equal(t, http.StatusUnprocessableEntity,
		perr.StatusOf(mustErr(svc.Create(ctx, companyScope, badSMS))))

	// R231: the same address twice is de-duplicated, not an insert conflict.
	dup := commsInput("Kanal", analyzerA)
	dup.Channels = []model.AlarmChannel{
		{Channel: model.NotifyChannelEmail, Target: "ops@example.com"},
		{Channel: model.NotifyChannelEmail, Target: "OPS@example.com"},
		{Channel: model.NotifyChannelSMS, Target: "+905551234567"},
	}
	got, err := svc.Create(ctx, companyScope, dup)
	require.NoError(t, err)
	require.Len(t, got.Channels, 2)
	require.Len(t, repo.channels[got.Alarm.ID], 2)

	tooMany := commsInput("Kanal", analyzerA)
	for i := range 51 {
		tooMany.Channels = append(tooMany.Channels,
			model.AlarmChannel{Channel: model.NotifyChannelEmail, Target: fmt.Sprintf("a%d@example.com", i)})
	}
	require.Equal(t, http.StatusUnprocessableEntity,
		perr.StatusOf(mustErr(svc.Create(ctx, companyScope, tooMany))))
}

func TestSMSTargetsAreStoredNotRejected(t *testing.T) {
	t.Parallel()
	// R211/D-1: the numbers are kept so a provider can be wired later; the
	// notify path is what refuses to pretend it sent anything.
	svc, repo := newTestService(t)
	in := commsInput("SMS", analyzerA)
	in.Channels = []model.AlarmChannel{{Channel: model.NotifyChannelSMS, Target: "+905551234567"}}
	got, err := svc.Create(context.Background(), companyScope, in)
	require.NoError(t, err)
	require.Len(t, repo.channels[got.Alarm.ID], 1)
	require.Equal(t, model.NotifyChannelSMS, got.Channels[0].Channel)
}

func TestUpdateIsAFullReplaceOfSettings(t *testing.T) {
	t.Parallel()
	// A threshold left out of a PATCH is CLEARED, which is what makes
	// "remove this limit" expressible at all.
	svc, _ := newTestService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, companyScope, alarms.Input{
		Name: "Reaktif", Type: model.AlarmTypeReactiveLimit, IsEnabled: true,
		Alarm: model.Alarm{InductiveRatioThreshold: dec("20"), InductivePeriodValue: i32(24),
			InductivePeriodUnit: unit(model.PeriodUnitHours), CapacitiveRatioThreshold: dec("15"),
			CapacitivePeriodValue: i32(24), CapacitivePeriodUnit: unit(model.PeriodUnitHours)},
		AnalyzerIDs: []uuid.UUID{analyzerA}})
	require.NoError(t, err)

	updated, err := svc.Update(ctx, companyScope, created.Alarm.ID, alarms.Input{
		Name: "Reaktif", Type: model.AlarmTypeReactiveLimit, IsEnabled: true,
		Alarm: model.Alarm{InductiveRatioThreshold: dec("30"), InductivePeriodValue: i32(12),
			InductivePeriodUnit: unit(model.PeriodUnitHours)},
		AnalyzerIDs: []uuid.UUID{analyzerA}})
	require.NoError(t, err)
	require.Equal(t, "30", updated.Alarm.InductiveRatioThreshold.String())
	require.Nil(t, updated.Alarm.CapacitiveRatioThreshold)
}

func TestUpdateAndDeleteRefuseARuleOutOfScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	ctx := context.Background()
	other, err := svc.Create(ctx, companyScope, commsInput("Başka bina", analyzerB))
	require.NoError(t, err)

	_, err = svc.Update(ctx, buildingScope, other.Alarm.ID, commsInput("Başka bina", analyzerA))
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, svc.Delete(ctx, buildingScope, other.Alarm.ID), store.ErrNotFound)
}

func TestEventsRefuseARuleOutOfScope(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	ctx := context.Background()
	other, err := svc.Create(ctx, companyScope, commsInput("Başka bina", analyzerB))
	require.NoError(t, err)

	_, err = svc.Events(ctx, buildingScope, other.Alarm.ID, store.AlarmEventFilter{})
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = svc.Events(ctx, companyScope, other.Alarm.ID, store.AlarmEventFilter{})
	require.NoError(t, err)
}

func TestListCarriesAnalyzerLabelsAndLastReading(t *testing.T) {
	t.Parallel()
	// The list column "applied analyzers" (§7.12) and the comms evaluation
	// (R216) both read these, so one load serves both.
	svc, _ := newTestService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, companyScope, commsInput("Etiket", analyzerA))
	require.NoError(t, err)
	require.Len(t, created.Analyzers, 1)
	require.NotEmpty(t, created.Analyzers[0].InstallationNumber)
	require.NotNil(t, created.Analyzers[0].LastReadingAt)
	require.Equal(t, buildingA, *created.Analyzers[0].BuildingID)
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	_, err := alarms.New(alarms.Deps{})
	require.Error(t, err)
	require.True(t, errors.Is(err, err), "a missing dependency must not build a Service")
}
