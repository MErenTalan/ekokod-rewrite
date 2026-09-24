package seed

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// ensureAlarms gives the e2e run one rule per alarm type, a delivered firing,
// a failed one, three operational messages (one per kind) and two job runs.
//
// It is idempotent like every other e2e seed step: a rule whose name already
// exists is left alone, so `ekokod seed e2e` can be re-run against a database
// that already has them.
func ensureAlarms(ctx context.Context, pool *pgxpool.Pool, f Fixtures, now time.Time) error {
	sc := store.SystemScope(f.CompanyA)
	repo := postgres.NewAlarmRepository(pool)

	existing, err := repo.List(ctx, sc, store.AlarmFilter{Page: store.Page{Limit: 50}})
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}

	dec := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
	i32 := func(v int32) *int32 { return &v }
	unit := func(u model.PeriodUnit) *model.PeriodUnit { return &u }

	rules := []struct {
		alarm    model.Alarm
		channels []model.AlarmChannel
	}{
		{
			alarm: model.Alarm{
				CompanyID: f.CompanyA, Name: "Endüktif izleme", Type: model.AlarmTypeReactiveLimit, IsEnabled: true,
				InductiveRatioThreshold: dec("20"), InductivePeriodValue: i32(24), InductivePeriodUnit: unit(model.PeriodUnitHours),
				NotificationFrequencyValue: i32(6), NotificationFrequencyUnit: unit(model.PeriodUnitHours),
				CreatedAt: now, UpdatedAt: now,
			},
			channels: []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "ops@e2e.ekokod.test"}},
		},
		{
			alarm: model.Alarm{
				CompanyID: f.CompanyA, Name: "İletişim kopukluğu", Type: model.AlarmTypeDataCommunication, IsEnabled: true,
				CommunicationThresholdHours: i32(6), CreatedAt: now, UpdatedAt: now,
			},
			// R211: an SMS target the screen must describe as not sent.
			channels: []model.AlarmChannel{
				{Channel: model.NotifyChannelEmail, Target: "ops@e2e.ekokod.test"},
				{Channel: model.NotifyChannelSMS, Target: "+905551112233"},
			},
		},
		{
			alarm: model.Alarm{
				CompanyID: f.CompanyA, Name: "Güç sınırı", Type: model.AlarmTypeCurrentVoltagePower, IsEnabled: false,
				PowerMax: dec("250"), CreatedAt: now, UpdatedAt: now,
			},
			channels: []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "ops@e2e.ekokod.test"}},
		},
		{
			alarm: model.Alarm{
				CompanyID: f.CompanyA, Name: "Fatura artışı", Type: model.AlarmTypeInvoiceIncrease, IsEnabled: true,
				InvoiceThresholdPct: dec("20"), CreatedAt: now, UpdatedAt: now,
			},
			channels: []model.AlarmChannel{{Channel: model.NotifyChannelEmail, Target: "ops@e2e.ekokod.test"}},
		},
	}

	var first uuid.UUID
	for i, rule := range rules {
		created, err := repo.Create(ctx, sc, rule.alarm)
		if err != nil {
			return err
		}
		if i == 0 {
			first = created.ID
		}
		if err := repo.ReplaceAnalyzers(ctx, sc, created.ID, []uuid.UUID{f.AnalyzerA1}); err != nil {
			return err
		}
		if err := repo.ReplaceChannels(ctx, sc, created.ID, rule.channels); err != nil {
			return err
		}
	}

	if err := seedEvents(ctx, repo, sc, first, f.AnalyzerA1, now); err != nil {
		return err
	}
	return seedOps(ctx, pool, sc, first, now)
}

// seedEvents writes one delivered firing and one that reached nobody, so the
// alarm log shows both states.
func seedEvents(ctx context.Context, repo *postgres.AlarmRepository, sc store.Scope,
	alarmID, analyzerID uuid.UUID, now time.Time,
) error {
	detail, err := json.Marshal(map[string]any{
		"analyzer": "E2E-A1", "alarm_type": string(model.AlarmTypeReactiveLimit),
		"lines": []string{"Endüktif oran %25 eşiği aştı (%20)"},
	})
	if err != nil {
		return err
	}
	delivered, err := repo.CreateEvent(ctx, sc, model.AlarmEvent{
		AlarmID: alarmID, AnalyzerID: &analyzerID, TriggeredAt: now.Add(-2 * time.Hour),
		Message: "Endüktif izleme — E2E-A1", Detail: detail,
	})
	if err != nil {
		return err
	}
	if err := repo.MarkNotified(ctx, sc, delivered.ID, now.Add(-2*time.Hour), nil); err != nil {
		return err
	}

	failed, err := repo.CreateEvent(ctx, sc, model.AlarmEvent{
		AlarmID: alarmID, AnalyzerID: &analyzerID, TriggeredAt: now.Add(-26 * time.Hour),
		Message: "Endüktif izleme — E2E-A1", Detail: detail,
	})
	if err != nil {
		return err
	}
	reason := "e-posta gönderilemedi: 535 authentication failed"
	return repo.MarkNotified(ctx, sc, failed.ID, time.Time{}, &reason)
}

// seedOps writes one message per kind and two job runs, so every filter on the
// Messages screen has something to include and something to exclude.
func seedOps(ctx context.Context, pool *pgxpool.Pool, sc store.Scope, alarmID uuid.UUID, now time.Time) error {
	ops := postgres.NewOpsRepository(pool)
	company := sc.CompanyID
	related := "alarm"
	detail := "Endüktif oran %25 eşiği aştı (%20)"

	for _, m := range []model.OperationalMessage{
		{CompanyID: &company, Kind: "alarm", Category: "alarm-trigger", Status: "warning",
			Message: "Alarm tetiklendi: Endüktif izleme", Detail: &detail,
			RelatedType: &related, RelatedID: &alarmID, CreatedAt: now.Add(-2 * time.Hour)},
		{CompanyID: &company, Kind: "job", Category: "alarm-evaluate", Status: "success",
			Message:   "Alarm değerlendirmesi tamamlandı (success): 4 kural işlendi, 3 tetiklenmedi, 0 hata, 1 bildirim.",
			CreatedAt: now.Add(-1 * time.Hour)},
		{CompanyID: &company, Kind: "system", Category: "auth", Status: "info",
			Message: "Oturum açıldı", CreatedAt: now.Add(-30 * time.Minute)},
	} {
		if _, err := ops.AppendMessage(ctx, sc, m); err != nil {
			return err
		}
	}

	for _, run := range []struct {
		jobType            string
		status             string
		processed, skipped int32
		failed             int32
		finished           bool
	}{
		{job.TypeAlarmEvaluate, "success", 4, 3, 0, true},
		{job.TypeBillingDispatch, "partial", 2, 0, 1, true},
	} {
		scope, err := json.Marshal(map[string]any{"company_id": company})
		if err != nil {
			return err
		}
		started, err := ops.StartRun(ctx, sc, model.JobRun{
			CompanyID: &company, JobType: run.jobType, Scope: scope, StartedAt: now.Add(-time.Hour),
		})
		if err != nil {
			return err
		}
		if !run.finished {
			continue
		}
		if _, err := ops.FinishRun(ctx, sc, started.ID, run.status,
			run.processed, run.skipped, run.failed, nil, nil, now.Add(-time.Hour).Add(12*time.Second)); err != nil {
			return err
		}
	}
	return nil
}
