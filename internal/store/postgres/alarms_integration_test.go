//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func alarmFixtureRow(companyID uuid.UUID) model.Alarm {
	now := time.Now().UTC()
	return model.Alarm{
		CompanyID: companyID, Name: "High reactive", Type: model.AlarmTypeReactiveLimit, IsEnabled: true,
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestAlarmGetIsScopedToCompanyOnly(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 400)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 401)
	repo := postgres.NewAlarmRepository(pool)

	a, err := repo.Create(ctx, tenantB.Scope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)

	// alarms carries no building_id: the whole company Scope (even the
	// narrow, building-limited one) can see it.
	_, err = repo.Get(ctx, tenantB.Scope, a.ID)
	require.NoError(t, err)

	// Another tenant cannot.
	_, err = repo.Get(ctx, tenantA.Scope, a.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Get(ctx, store.Scope{}, a.ID)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestAlarmAnalyzersJoinThroughAlarmIsolation covers the two mandatory
// methods on alarm_analyzers: Analyzers and ReplaceAnalyzers must join
// through alarms, and every analyzer id in ReplaceAnalyzers must itself be
// visible to the Scope.
//
// Deliberate-break proof (reverted): dropping the AlarmCountVisibleAnalyzers
// check from ReplaceAnalyzers let a narrow Scope attach an analyzer from
// Buildings[1] (outside it) without error, instead of ErrNotFound.
func TestAlarmAnalyzersJoinThroughAlarmIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 410)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 411)
	repo := postgres.NewAlarmRepository(pool)

	alarmB, err := repo.Create(ctx, tenantB.AdminScope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)

	_, err = repo.Analyzers(ctx, tenantA.AdminScope, alarmB.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	err = repo.ReplaceAnalyzers(ctx, tenantA.AdminScope, alarmB.ID, []uuid.UUID{tenantB.Analyzers[0].ID})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant B, its own alarm and its own analyzer: works.
	err = repo.ReplaceAnalyzers(ctx, tenantB.AdminScope, alarmB.ID, []uuid.UUID{tenantB.Analyzers[0].ID, tenantB.Analyzers[1].ID})
	require.NoError(t, err)
	list, err := repo.Analyzers(ctx, tenantB.AdminScope, alarmB.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)

	// A narrow Scope (Buildings[0] only) may attach analyzers of its own
	// building but not of Buildings[1].
	err = repo.ReplaceAnalyzers(ctx, tenantB.Scope, alarmB.ID, []uuid.UUID{tenantB.Analyzers[2].ID}) // Buildings[1]
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestAlarmChannelsJoinThroughAlarmIsolation covers the two mandatory
// methods on alarm_channels.
func TestAlarmChannelsJoinThroughAlarmIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 420)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 421)
	repo := postgres.NewAlarmRepository(pool)

	alarmB, err := repo.Create(ctx, tenantB.AdminScope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)

	_, err = repo.Channels(ctx, tenantA.AdminScope, alarmB.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	err = repo.ReplaceChannels(ctx, tenantA.AdminScope, alarmB.ID, []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "a@b.invalid"}})
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.ReplaceChannels(ctx, tenantB.AdminScope, alarmB.ID, []model.AlarmChannel{
		{Channel: model.NotifyChannelEmail, Target: "ops@tenant.invalid"},
		{Channel: model.NotifyChannelSMS, Target: "+905551234567"},
	})
	require.NoError(t, err)
	got, err := repo.Channels(ctx, tenantB.AdminScope, alarmB.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
}

// TestAlarmEventsJoinThroughAlarmIsolation covers the three mandatory
// methods on alarm_events: CreateEvent, ListEvents and MarkNotified.
//
// Deliberate-break proof (reverted): replacing requireVisible's body with
// `return nil` in alarms.go let CreateEvent, ListEvents and MarkNotified all
// succeed against tenant B's alarm from tenant A's Scope.
func TestAlarmEventsJoinThroughAlarmIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 430)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 431)
	repo := postgres.NewAlarmRepository(pool)

	alarmB, err := repo.Create(ctx, tenantB.AdminScope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)

	_, err = repo.CreateEvent(ctx, tenantA.AdminScope, model.AlarmEvent{AlarmID: alarmB.ID, TriggeredAt: time.Now().UTC(), Message: "fired"})
	require.ErrorIs(t, err, store.ErrNotFound)

	ev, err := repo.CreateEvent(ctx, tenantB.AdminScope, model.AlarmEvent{
		AlarmID: alarmB.ID, AnalyzerID: &tenantB.Analyzers[0].ID, TriggeredAt: time.Now().UTC(), Message: "fired",
	})
	require.NoError(t, err)

	// An analyzer belonging to another tenant is refused even for the
	// owning alarm.
	_, err = repo.CreateEvent(ctx, tenantB.AdminScope, model.AlarmEvent{
		AlarmID: alarmB.ID, AnalyzerID: &tenantA.Analyzers[0].ID, TriggeredAt: time.Now().UTC(), Message: "fired",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	// No error: an alarm id not visible to the Scope simply contributes no
	// rows to ListEvents, rather than being reported as not found.
	listedForA, err := repo.ListEvents(ctx, tenantA.AdminScope, store.AlarmEventFilter{AlarmID: &alarmB.ID})
	require.NoError(t, err)
	require.Empty(t, listedForA, "tenant A must never see tenant B's alarm events")

	listedForB, err := repo.ListEvents(ctx, tenantB.AdminScope, store.AlarmEventFilter{AlarmID: &alarmB.ID})
	require.NoError(t, err)
	require.Len(t, listedForB, 1)

	err = repo.MarkNotified(ctx, tenantA.AdminScope, ev.ID, time.Now().UTC(), nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.MarkNotified(ctx, tenantB.AdminScope, ev.ID, time.Now().UTC(), nil)
	require.NoError(t, err)
}

// TestAlarmMarkBillFiredJoinsThroughAlarmsAndBills covers the mandatory
// method on alarm_fired_bills: BOTH the alarm and the bill must be visible.
//
// Deliberate-break proof (reverted): removing the BillVisible check from
// MarkBillFired let tenant A's Scope claim a (tenant-A-alarm,
// tenant-B-bill) pair, when it should be refused because the bill is not
// tenant A's.
func TestAlarmMarkBillFiredJoinsThroughAlarmsAndBills(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 440)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 441)
	alarmRepo := postgres.NewAlarmRepository(pool)
	billRepo := postgres.NewBillRepository(pool)

	alarmA, err := alarmRepo.Create(ctx, tenantA.AdminScope, alarmFixtureRow(tenantA.Company.ID))
	require.NoError(t, err)
	buildingB := tenantB.Buildings[0].ID
	billB, err := billRepo.Create(ctx, tenantB.Scope, billFixtureRow(tenantB.Company.ID, &buildingB, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	_, err = alarmRepo.MarkBillFired(ctx, tenantA.AdminScope, alarmA.ID, billB.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "tenant A's alarm cannot be linked to tenant B's bill")

	buildingA := tenantA.Buildings[0].ID
	billA, err := billRepo.Create(ctx, tenantA.Scope, billFixtureRow(tenantA.Company.ID, &buildingA, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	claimed, err := alarmRepo.MarkBillFired(ctx, tenantA.AdminScope, alarmA.ID, billA.ID)
	require.NoError(t, err)
	require.True(t, claimed)

	// Claiming the same pair again reports it was already claimed.
	claimedAgain, err := alarmRepo.MarkBillFired(ctx, tenantA.AdminScope, alarmA.ID, billA.ID)
	require.NoError(t, err)
	require.False(t, claimedAgain)
}

// TestAlarmAnalyzersAndEventsHideBuildingOutsideNarrowScope is Important
// Finding 2's probe: the alarm RULE is company-wide (accepted), but its
// building-scoped children are not. A narrow Scope on Buildings[0] must
// never learn — through Analyzers, ListEvents, or a AnalyzerID List filter —
// that the SAME alarm also watches an analyzer under Buildings[1].
func TestAlarmAnalyzersAndEventsHideBuildingOutsideNarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 460)
	repo := postgres.NewAlarmRepository(pool)

	alarm, err := repo.Create(ctx, tenant.AdminScope, alarmFixtureRow(tenant.Company.ID))
	require.NoError(t, err)

	visibleAnalyzer := tenant.Analyzers[0].ID // Buildings[0]
	hiddenAnalyzer := tenant.Analyzers[2].ID  // Buildings[1]
	require.NoError(t, repo.ReplaceAnalyzers(ctx, tenant.AdminScope, alarm.ID, []uuid.UUID{visibleAnalyzer, hiddenAnalyzer}))

	// Analyzers(): the narrow Scope sees only its own building's attachment.
	list, err := repo.Analyzers(ctx, tenant.Scope, alarm.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, visibleAnalyzer, list[0].AnalyzerID)

	// ListEvents(): an event on the HIDDEN analyzer, and its message/detail
	// payload, must never reach the narrow Scope.
	hiddenEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, AnalyzerID: &hiddenAnalyzer, TriggeredAt: time.Now().UTC(), Message: "secret",
	})
	require.NoError(t, err)
	visibleEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, AnalyzerID: &visibleAnalyzer, TriggeredAt: time.Now().UTC(), Message: "public",
	})
	require.NoError(t, err)

	events, err := repo.ListEvents(ctx, tenant.Scope, store.AlarmEventFilter{AlarmID: &alarm.ID})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, visibleEvent.ID, events[0].ID)
	for _, e := range events {
		require.NotEqual(t, hiddenEvent.ID, e.ID, "the narrow Scope must never see the hidden-building event")
	}

	// A company-level event (NULL analyzer_id) is visible only to AllBuildings.
	companyEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, TriggeredAt: time.Now().UTC(), Message: "company-wide",
	})
	require.NoError(t, err)
	narrowAfter, err := repo.ListEvents(ctx, tenant.Scope, store.AlarmEventFilter{AlarmID: &alarm.ID})
	require.NoError(t, err)
	for _, e := range narrowAfter {
		require.NotEqual(t, companyEvent.ID, e.ID, "a narrow Scope must never see a NULL-analyzer alarm event")
	}
	adminAfter, err := repo.ListEvents(ctx, tenant.AdminScope, store.AlarmEventFilter{AlarmID: &alarm.ID})
	require.NoError(t, err)
	require.Len(t, adminAfter, 3)

	// AlarmList's AnalyzerID filter narrows the same way: filtering by the
	// hidden analyzer must not surface the alarm to the narrow Scope.
	byHidden, err := repo.List(ctx, tenant.Scope, store.AlarmFilter{AnalyzerID: &hiddenAnalyzer})
	require.NoError(t, err)
	require.Empty(t, byHidden, "the narrow Scope must not learn the alarm is attached via a hidden analyzer")
	byVisible, err := repo.List(ctx, tenant.Scope, store.AlarmFilter{AnalyzerID: &visibleAnalyzer})
	require.NoError(t, err)
	require.Len(t, byVisible, 1)
}

// TestAlarmMarkNotifiedRespectsNarrowScopeAndSoftDelete is F1 final review
// pass A, Important Finding 3's probe. Pre-fix, MarkNotified checked only
// alarms.company_id: a narrow Scope could mark notified an event on an
// analyzer outside its buildings, or a NULL-analyzer (company-level) event
// it could never list, and a soft-deleted alarm's events stayed both
// listable and markable under AdminScope. MarkNotified now applies
// ListEvents' exact visibility rule and excludes a soft-deleted alarm's
// children, so nothing reachable by one method is unreachable by the other.
func TestAlarmMarkNotifiedRespectsNarrowScopeAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 461)
	repo := postgres.NewAlarmRepository(pool)

	alarm, err := repo.Create(ctx, tenant.AdminScope, alarmFixtureRow(tenant.Company.ID))
	require.NoError(t, err)

	visibleAnalyzer := tenant.Analyzers[0].ID // Buildings[0], in tenant.Scope
	hiddenAnalyzer := tenant.Analyzers[2].ID  // Buildings[1], outside tenant.Scope
	require.NoError(t, repo.ReplaceAnalyzers(ctx, tenant.AdminScope, alarm.ID, []uuid.UUID{visibleAnalyzer, hiddenAnalyzer}))

	hiddenEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, AnalyzerID: &hiddenAnalyzer, TriggeredAt: time.Now().UTC(), Message: "hidden",
	})
	require.NoError(t, err)
	visibleEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, AnalyzerID: &visibleAnalyzer, TriggeredAt: time.Now().UTC(), Message: "visible",
	})
	require.NoError(t, err)
	companyEvent, err := repo.CreateEvent(ctx, tenant.AdminScope, model.AlarmEvent{
		AlarmID: alarm.ID, TriggeredAt: time.Now().UTC(), Message: "company-wide",
	})
	require.NoError(t, err)

	// A narrow Scope must not be able to mark notified an event it could
	// never list: the hidden-building event and the NULL-analyzer
	// company-level event both refuse.
	err = repo.MarkNotified(ctx, tenant.Scope, hiddenEvent.ID, time.Now().UTC(), nil)
	require.ErrorIs(t, err, store.ErrNotFound, "narrow Scope must not mark notified an event on a hidden-building analyzer")
	err = repo.MarkNotified(ctx, tenant.Scope, companyEvent.ID, time.Now().UTC(), nil)
	require.ErrorIs(t, err, store.ErrNotFound, "narrow Scope must not mark notified a NULL-analyzer (company-level) event")

	// Positive controls: the narrow Scope CAN mark notified its own visible
	// event, and AllBuildings can mark notified the company-level one.
	require.NoError(t, repo.MarkNotified(ctx, tenant.Scope, visibleEvent.ID, time.Now().UTC(), nil),
		"positive control: narrow Scope must be able to mark notified its own visible-analyzer event")
	require.NoError(t, repo.MarkNotified(ctx, tenant.AdminScope, companyEvent.ID, time.Now().UTC(), nil),
		"positive control: AllBuildings must be able to mark notified a NULL-analyzer event")

	// Soft-deleting the alarm makes ALL its events unreachable, by BOTH
	// ListEvents and MarkNotified, even under AdminScope.
	require.NoError(t, repo.SoftDelete(ctx, tenant.AdminScope, alarm.ID, time.Now().UTC()))

	afterDelete, err := repo.ListEvents(ctx, tenant.AdminScope, store.AlarmEventFilter{AlarmID: &alarm.ID})
	require.NoError(t, err)
	require.Empty(t, afterDelete, "a soft-deleted alarm's events must not be listed, even under AdminScope")

	err = repo.MarkNotified(ctx, tenant.AdminScope, hiddenEvent.ID, time.Now().UTC(), nil)
	require.ErrorIs(t, err, store.ErrNotFound, "a soft-deleted alarm's event must not be markable notified, even under AdminScope")
}

// TestAlarmReplaceAnalyzersLeavesInvisibleAttachmentsUntouched is Important
// Finding 2's probe on the write side: ReplaceAnalyzers must replace ONLY
// the attachments visible to the Scope, not silently detach one outside it.
func TestAlarmReplaceAnalyzersLeavesInvisibleAttachmentsUntouched(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 461)
	repo := postgres.NewAlarmRepository(pool)

	alarm, err := repo.Create(ctx, tenant.AdminScope, alarmFixtureRow(tenant.Company.ID))
	require.NoError(t, err)

	hiddenAnalyzer := tenant.Analyzers[2].ID // Buildings[1]
	require.NoError(t, repo.ReplaceAnalyzers(ctx, tenant.AdminScope, alarm.ID, []uuid.UUID{hiddenAnalyzer}))

	// The narrow Scope replaces its OWN (empty) list of visible attachments.
	visibleAnalyzer := tenant.Analyzers[0].ID // Buildings[0]
	require.NoError(t, repo.ReplaceAnalyzers(ctx, tenant.Scope, alarm.ID, []uuid.UUID{visibleAnalyzer}))

	// Both attachments survive: the hidden one was never touched.
	got, err := repo.Analyzers(ctx, tenant.AdminScope, alarm.ID)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, aa := range got {
		ids = append(ids, aa.AnalyzerID)
	}
	require.ElementsMatch(t, []uuid.UUID{hiddenAnalyzer, visibleAnalyzer}, ids,
		"a narrow Scope's ReplaceAnalyzers must not have detached the hidden-building analyzer")
}

// TestAlarmUpdateRejectsForeignCompanyID is Important Finding 3's probe.
func TestAlarmUpdateRejectsForeignCompanyID(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 462)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 463)
	repo := postgres.NewAlarmRepository(pool)

	created, err := repo.Create(ctx, tenantA.AdminScope, alarmFixtureRow(tenantA.Company.ID))
	require.NoError(t, err)

	tampered := created
	tampered.CompanyID = tenantB.Company.ID
	_, err = repo.Update(ctx, tenantA.AdminScope, tampered)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestAlarmCrossTenantUpdateAndSoftDelete is Important Finding 7's missing
// test: cross-tenant Update and SoftDelete for alarms.
func TestAlarmCrossTenantUpdateAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 464)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 465)
	repo := postgres.NewAlarmRepository(pool)

	alarmB, err := repo.Create(ctx, tenantB.AdminScope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)

	tampered := alarmB
	tampered.Name = "renamed by tenant A"
	_, err = repo.Update(ctx, tenantA.AdminScope, tampered)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.SoftDelete(ctx, tenantA.AdminScope, alarmB.ID, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Get(ctx, tenantB.AdminScope, alarmB.ID)
	require.NoError(t, err)
	require.Nil(t, got.DeletedAt)
}

// TestAlarmMarkBillFiredRejectsInvisibleAlarm is Important Finding 7's
// missing test: MarkBillFired with an invisible ALARM (as opposed to the
// existing invisible-BILL case).
func TestAlarmMarkBillFiredRejectsInvisibleAlarm(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 466)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 467)
	alarmRepo := postgres.NewAlarmRepository(pool)
	billRepo := postgres.NewBillRepository(pool)

	alarmB, err := alarmRepo.Create(ctx, tenantB.AdminScope, alarmFixtureRow(tenantB.Company.ID))
	require.NoError(t, err)
	buildingA := tenantA.Buildings[0].ID
	billA, err := billRepo.Create(ctx, tenantA.Scope, billFixtureRow(tenantA.Company.ID, &buildingA, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	_, err = alarmRepo.MarkBillFired(ctx, tenantA.AdminScope, alarmB.ID, billA.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "tenant A cannot claim firing for tenant B's alarm even against its OWN bill")
}

// TestAlarmPageLimitsClampNegativeOffset is the folded-minor probe: OFFSET
// must not be negative.
func TestAlarmPageLimitsClampNegativeOffset(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 468)
	repo := postgres.NewAlarmRepository(pool)

	_, err := repo.List(ctx, tenant.AdminScope, store.AlarmFilter{Page: store.Page{Limit: 10, Offset: -1}})
	require.NoError(t, err, "a negative Offset must be clamped to 0")
}

// TestAlarmMarkIsolarForwardedJoinsThroughPlant covers the mandatory method
// on isolar_forwarded_alarms.
func TestAlarmMarkIsolarForwardedJoinsThroughPlant(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 450)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 451)
	repo := postgres.NewAlarmRepository(pool)

	_, err := repo.MarkIsolarForwarded(ctx, tenantA.AdminScope, tenantB.Plants[0].ID, "ref-1", time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound, "tenant A cannot claim forwarding for tenant B's plant")

	claimed, err := repo.MarkIsolarForwarded(ctx, tenantB.AdminScope, tenantB.Plants[0].ID, "ref-1", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, claimed)

	claimedAgain, err := repo.MarkIsolarForwarded(ctx, tenantB.AdminScope, tenantB.Plants[0].ID, "ref-1", time.Now().UTC())
	require.NoError(t, err)
	require.False(t, claimedAgain)
}
