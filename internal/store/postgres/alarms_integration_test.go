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
