package seed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	carbondomain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	carbonsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	reportsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// ensureDemoContent fills the remaining screens: alarms with a month of
// firings, job history, anomalies, forecasts, reports, the ISO 50001 project
// and the EKOCM records. Each part is skipped when its rows already exist.
func ensureDemoContent(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	for _, step := range []func(context.Context, DemoDeps, demoBaseline, time.Time) error{
		ensureDemoAlarms, ensureDemoJobRuns, ensureDemoAnomalies, ensureDemoForecasts, ensureDemoReports,
		ensureDemoISO, ensureDemoCarbon,
	} {
		if err := step(ctx, d, b, now); err != nil {
			return err
		}
	}
	return nil
}

type demoAlarm struct {
	alarm     model.Alarm
	analyzers []int // baseline analyzer indexes; nil = all
	weight    int   // firings per round of the month's deck
	values    []string
	line      string
}

func demoAlarms(b demoBaseline, now time.Time) []demoAlarm {
	dec := func(s string) *decimal.Decimal { return ptr(decimal.RequireFromString(s)) }
	hours := ptr(model.PeriodUnitHours)
	base := func(name string, t model.AlarmType) model.Alarm {
		return model.Alarm{CompanyID: b.company, Name: name, Type: t, IsEnabled: true, NotificationFrequencyValue: ptr(int32(6)),
			NotificationFrequencyUnit: hours, CreatedAt: now, UpdatedAt: now}
	}
	power := base("Fabrika güç aşımı", model.AlarmTypeCurrentVoltagePower)
	power.PowerMax = dec("380")
	daily := base("Günlük tüketim sınırı", model.AlarmTypeReactiveLimit)
	daily.ActiveConsumptionMax, daily.ActiveConsumptionMaxPeriodValue, daily.ActiveConsumptionMaxPeriodUnit = dec("900"), ptr(int32(24)), hours
	reactive := base("Reaktif oran", model.AlarmTypeReactiveLimit)
	reactive.InductiveRatioThreshold, reactive.InductivePeriodValue, reactive.InductivePeriodUnit = dec("20"), ptr(int32(24)), hours
	invoice := base("Fatura artışı", model.AlarmTypeInvoiceIncrease)
	invoice.InvoiceThresholdPct = dec("15")
	silent := base("Veri gelmiyor", model.AlarmTypeDataCommunication)
	silent.CommunicationThresholdHours = ptr(int32(6))
	return []demoAlarm{
		{power, []int{0}, 6, []string{"392", "405", "387"}, "Anlık güç %s kW, sınır 380 kW"},
		{daily, []int{3}, 10, []string{"948", "1012", "931"}, "Günlük tüketim %s kWh, sınır 900 kWh"},
		{reactive, []int{2}, 14, []string{"26", "31", "23"}, "Endüktif oran %%%s, sınır %%20"},
		{invoice, []int{0, 1, 2}, 3, []string{"18", "22", "16"}, "Fatura tutarı önceki aya göre %%%s arttı (sınır %%15)"},
		{silent, nil, 7, []string{"9", "7", "12"}, "%s saattir veri gelmiyor (sınır 6 saat)"},
	}
}

// ensureDemoAlarms writes five rules and 40 firings over the last 30 days,
// each with the operational message the evaluator would have written.
func ensureDemoAlarms(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo, ops := postgres.NewAlarmRepository(d.Pool), postgres.NewOpsRepository(d.Pool)
	have, err := repo.List(ctx, sc, store.AlarmFilter{Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	rules := demoAlarms(b, now)
	var deck []int // rule index per firing, interleaved by weight
	for round := 0; len(deck) < 40; round++ {
		for i, r := range rules {
			if round < r.weight && len(deck) < 40 {
				deck = append(deck, i)
			}
		}
	}
	ids := make([]uuid.UUID, len(rules))
	for i, r := range rules {
		created, err := repo.Create(ctx, sc, r.alarm)
		if err != nil {
			return fmt.Errorf("seed demo alarm %s: %w", r.alarm.Name, err)
		}
		ids[i] = created.ID
		analyzers := b.Keep().Analyzers
		if r.analyzers != nil {
			analyzers = nil
			for _, k := range r.analyzers {
				analyzers = append(analyzers, b.analyzers[k].id)
			}
		}
		if err := repo.ReplaceAnalyzers(ctx, sc, created.ID, analyzers); err != nil {
			return err
		}
		if err := repo.ReplaceChannels(ctx, sc, created.ID, []model.AlarmChannel{
			{Channel: model.NotifyChannelEmail, Target: "enerji@demo-enerji.example"}}); err != nil {
			return err
		}
	}
	company, related := b.company, "alarm"
	for k, i := range deck {
		r := rules[i]
		pick := r.analyzers
		if pick == nil {
			pick = []int{k % len(b.analyzers)}
		}
		a := b.analyzers[pick[k%len(pick)]]
		// Newest first, about 18 hours apart, so the month reads as steady traffic.
		at := now.Add(-time.Duration(k*18+int(mulberry32(uint32(k))%12)+1) * time.Hour).Truncate(time.Minute)
		line := fmt.Sprintf(r.line, r.values[k%len(r.values)])
		detail, err := json.Marshal(map[string]any{"analyzer": a.number, "alarm_type": string(r.alarm.Type), "lines": []string{line}})
		if err != nil {
			return err
		}
		event, err := repo.CreateEvent(ctx, sc, model.AlarmEvent{AlarmID: ids[i], AnalyzerID: &a.id, TriggeredAt: at,
			Message: r.alarm.Name + " — " + a.number, Detail: detail})
		if err != nil {
			return err
		}
		notified, failure := at.Add(time.Minute), (*string)(nil)
		if k%17 == 5 {
			notified, failure = time.Time{}, ptr("e-posta gönderilemedi: alıcı sunucusu yanıt vermedi")
		}
		if err := repo.MarkNotified(ctx, sc, event.ID, notified, failure); err != nil {
			return err
		}
		if _, err := ops.AppendMessage(ctx, sc, model.OperationalMessage{CompanyID: &company, Kind: "alarm", Category: "alarm-trigger",
			Status: "warning", Message: "Alarm tetiklendi: " + r.alarm.Name, Detail: &line, RelatedType: &related, RelatedID: &ids[i],
			CreatedAt: at}); err != nil {
			return err
		}
	}
	return nil
}

// ensureDemoJobRuns writes the job history the Messages tab shows: a day of
// hourly fetches (one partial) and three months of billing and report runs.
func ensureDemoJobRuns(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	var n int
	if err := d.Pool.QueryRow(ctx, `select count(*) from job_runs where company_id = $1 and job_type = $2`, b.company,
		job.TypeIntegrationFetchReadings).Scan(&n); err != nil || n > 0 {
		return err
	}
	ops, company := postgres.NewOpsRepository(d.Pool), b.company
	type run struct {
		jobType                    string
		at                         time.Time
		status                     string
		processed, skipped, failed int32
		errText                    *string
		took                       time.Duration
	}
	var runs []run
	top := now.Truncate(time.Hour)
	for h := 1; h <= 24; h++ {
		r := run{job.TypeIntegrationFetchReadings, top.Add(-time.Duration(h)*time.Hour + 5*time.Minute), "success", int32(len(b.analyzers)), 0, 0, nil, 9 * time.Second}
		if h == 17 {
			r.status, r.processed, r.failed = "partial", int32(len(b.analyzers)-1), 1
			r.errText = ptr("DEMO-3002: OSOS yanıt vermedi, bir sonraki çalıştırmada yeniden denenecek")
		}
		runs = append(runs, r)
	}
	local := now.In(istanbul)
	for m := 0; m < 3; m++ {
		first := time.Date(local.Year(), local.Month()-time.Month(m), 1, 3, 0, 0, 0, istanbul)
		runs = append(runs,
			run{job.TypeBillingDispatch, first, "success", int32(len(b.analyzers) + len(b.buildings)), 0, 0, nil, 40 * time.Second},
			run{job.TypeReportDispatchMonthly, first.Add(3 * time.Hour), "success", int32(len(b.buildings)), 0, 0, nil, 25 * time.Second})
	}
	scope, err := json.Marshal(map[string]any{"company_id": company})
	if err != nil {
		return err
	}
	for _, r := range runs {
		started, err := ops.StartRun(ctx, sc, model.JobRun{CompanyID: &company, JobType: r.jobType, Scope: scope, StartedAt: r.at.UTC(),
			Status: "running"})
		if err != nil {
			return fmt.Errorf("seed demo job run: %w", err)
		}
		if _, err := ops.FinishRun(ctx, sc, started.ID, r.status, r.processed, r.skipped, r.failed, r.errText, nil,
			r.at.Add(r.took).UTC()); err != nil {
			return err
		}
	}
	for _, m := range []model.OperationalMessage{
		{Kind: "job", Category: "integration-fetch", Status: "warning", Message: "Okuma çekme kısmen tamamlandı: 5 sayaç güncellendi, 1 sayaç yanıt vermedi.",
			CreatedAt: top.Add(-17*time.Hour + 5*time.Minute)},
		{Kind: "system", Category: "bill-generation", Status: "success", Message: "Aylık faturalar hesaplandı: 9 fatura oluşturuldu.",
			CreatedAt: time.Date(local.Year(), local.Month(), 1, 3, 1, 0, 0, istanbul)},
		{Kind: "system", Category: "auth", Status: "info", Message: "Oturum açıldı", CreatedAt: now.Add(-20 * time.Minute)},
	} {
		m.CompanyID = &company
		if _, err := ops.AppendMessage(ctx, sc, m); err != nil {
			return err
		}
	}
	return nil
}

// ensureDemoAnomalies records six suspect periods on the factory meters over
// the last 90 days; the two oldest are resolved as accepted.
func ensureDemoAnomalies(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewAnomalyRepository(d.Pool)
	have, err := repo.List(ctx, sc, store.AnomalyFilter{Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	day := istanbulDay(now)
	for i, x := range []struct {
		analyzer, daysBack, hours int
		reason                    string
	}{
		{0, 84, 3, "meter_reset"}, {1, 71, 2, "negative_delta"}, {0, 52, 6, "missing_readings"},
		{1, 33, 2, "negative_delta"}, {0, 17, 4, "missing_readings"}, {1, 6, 3, "meter_reset"},
	} {
		from := day.AddDate(0, 0, -x.daysBack).Add(time.Duration(9+i) * time.Hour).UTC()
		to := from.Add(time.Duration(x.hours) * time.Hour)
		register := map[string]any{"reason": x.reason, "reset_rows": 0}
		switch x.reason {
		case "negative_delta":
			register["delta"] = "-128.400"
		case "meter_reset":
			register["reset_rows"] = 1
		}
		raw, err := json.Marshal(map[string]any{"code": "consumption.suspect_period", "period_start": from.Format(time.RFC3339),
			"period_end": to.Format(time.RFC3339), "registers": map[string]any{"active_import": register}})
		if err != nil {
			return err
		}
		a, err := repo.Create(ctx, sc, model.ConsumptionAnomaly{AnalyzerID: b.analyzers[x.analyzer].id, PeriodStart: from, PeriodEnd: to,
			Reason: x.reason, Detail: raw, CreatedAt: to.Add(time.Hour)})
		if err != nil {
			return fmt.Errorf("seed demo anomaly: %w", err)
		}
		if i < 2 {
			if _, err := repo.Resolve(ctx, sc, a.ID, DemoUserID, "accepted", nil, to.Add(26*time.Hour)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureDemoForecasts writes an hourly forecast for the last 30 and next 7
// days of every analyzer, for the hours that have none yet. All rows share
// one generated_at, reusing a stored one, so the latest-run view shows them
// as one run.
func ensureDemoForecasts(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewForecastRepository(d.Pool)
	hour := now.UTC().Truncate(time.Hour)
	from, to := hour.Add(-30*24*time.Hour), hour.Add(7*24*time.Hour)
	for i, a := range b.analyzers {
		have, err := repo.Range(ctx, sc, a.id, store.TimeRange{From: from, To: to.Add(time.Hour)})
		if err != nil {
			return err
		}
		run := istanbulDay(now).UTC()
		if len(have) > 0 {
			run = have[0].GeneratedAt
		}
		seen := make(map[int64]bool, len(have))
		for _, f := range have {
			seen[f.Ts.Unix()] = true
			if f.GeneratedAt.After(run) {
				run = f.GeneratedAt
			}
		}
		var rows []model.Forecast
		for ts := from; !ts.After(to); ts = ts.Add(time.Hour) {
			if seen[ts.Unix()] {
				continue
			}
			seed := uint32(ts.Unix()/3600)*2654435761 + uint32(i+1)*40503
			load := demoLoad(a, ts, seed)
			if a.solarPlant >= 0 {
				load = max(0, load-solarKwh(b.plants[a.solarPlant], ts))
			}
			median := decimal.NewFromFloat(load * (0.97 + 0.06*unit(mulberry32(seed+7)))).Round(3)
			rows = append(rows, model.Forecast{AnalyzerID: a.id, Ts: ts, GeneratedAt: run, HorizonHours: int32(ts.Sub(run) / time.Hour),
				Median: median, P10: ptr(median.Mul(decimal.RequireFromString("0.85")).Round(3)),
				P90: ptr(median.Mul(decimal.RequireFromString("1.15")).Round(3)), ModelID: "demo-baseline", ModelVersion: ptr("1")})
		}
		if len(rows) == 0 {
			continue
		}
		if _, _, err := repo.BulkInsert(ctx, sc, rows); err != nil {
			return fmt.Errorf("seed demo forecasts %s: %w", a.number, err)
		}
	}
	return nil
}

// ensureDemoReports builds the last three monthly reports of every building
// and last year's yearly report of the factory with the report service.
func ensureDemoReports(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	svc, err := reportService(d.Pool, now)
	if err != nil {
		return err
	}
	reports := postgres.NewReportRepository(d.Pool)
	local := now.In(istanbul)
	type want struct {
		building demoBuilding
		kind     model.ReportType
		period   string
	}
	var wants []want
	for m := 1; m <= 3; m++ {
		period := time.Date(local.Year(), local.Month()-time.Month(m), 1, 0, 0, 0, 0, istanbul).Format("2006-01")
		for _, x := range b.buildings {
			wants = append(wants, want{x, model.ReportTypeMonthly, period})
		}
	}
	wants = append(wants, want{b.buildings[0], model.ReportTypeYearly, fmt.Sprint(local.Year() - 1)})
	for _, w := range wants {
		have, err := reports.List(ctx, sc, store.ReportFilter{BuildingID: &w.building.id, Type: &w.kind, Period: &w.period, Page: store.Page{Limit: 1}})
		if err != nil {
			return err
		}
		if len(have) > 0 {
			continue
		}
		payload, err := svc.Build(ctx, sc, reportsvc.Request{Type: string(w.kind), Period: w.period, Selection: domain.SelectionAll,
			BuildingIDs: []uuid.UUID{w.building.id}})
		if err != nil {
			return fmt.Errorf("seed demo report %s %s: %w", w.building.name, w.period, err)
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		subject, body, err := reportsvc.EmailContent(payload, []string{w.building.name}, "tr")
		if err != nil {
			return err
		}
		processed := now.UTC()
		if _, err := reports.Upsert(ctx, sc, model.Report{CompanyID: b.company, BuildingID: w.building.id, Type: w.kind, Period: w.period,
			PlantSelection: model.PlantSelectionAll, Payload: raw, Status: model.ReportStatusCompleted, EmailSubject: &subject,
			EmailBody: &body, ProcessedAt: &processed, CreatedAt: processed}); err != nil {
			return err
		}
	}
	return nil
}

// demoDocuments are the ISO 50001 files the factory's project carries.
var demoDocuments = []struct {
	clause, name, title string
	lines               []string
}{
	{"5.2", "enerji-politikasi.pdf", "Enerji Politikası", []string{
		"Demo Enerji A.Ş., enerji performansını sürekli iyileştirmeyi taahhüt eder.", //nolint:misspell // Turkish
		"Enerji hedeflerimizi her yıl gözden geçirir, gerekli kaynakları sağlarız.",
		"Enerji verimli ürün ve hizmet satın alımını destekleriz.",
		"Yasal şartlara ve ISO 50001 gerekliliklerine uyarız."}},
	{"6.3", "enerji-gozden-gecirme.pdf", "Enerji Gözden Geçirme Raporu", []string{
		"Önemli enerji kullanımları: üretim hattı, kompresör dairesi, aydınlatma.",
		"Kompresör dairesinde yaz aylarında endüktif oran %20 sınırını aşmaktadır.",
		"İyileştirme fırsatı: kompanzasyon panosunun yenilenmesi.",
		"Hedef: bir önceki yıla göre %8 enerji tasarrufu."}},
	{"9.1", "enerji-performans-izleme.pdf", "Enerji Performansı İzleme Planı", []string{ //nolint:misspell // Turkish
		"Enerji performans göstergeleri: kWh/ton ürün, kWh/m², reaktif oran.", //nolint:misspell // Turkish
		"Ölçüm sıklığı: saatlik sayaç verisi, aylık fatura karşılaştırması.",
		"Sorumlular: enerji yöneticisi ve bakım şefi.",
		"Sonuçlar üç ayda bir yönetimin gözden geçirmesine sunulur."}},
}

// ensureDemoISO writes the factory's ISO 50001 project: clauses 5 and 6 done,
// 7 running, 8 and 9 ahead, four notes and three documents.
func ensureDemoISO(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewISO50001Repository(d.Pool)
	factory := b.buildings[0].id
	p, err := repo.EnsureProject(ctx, sc, factory)
	if err != nil {
		return fmt.Errorf("seed demo iso50001 project: %w", err)
	}
	y, m, _ := now.In(istanbul).Date()
	month := func(k int) time.Time { return time.Date(y, m+time.Month(k), 1, 0, 0, 0, 0, time.UTC) }
	var dates []model.ISO50001ClauseDate
	for i, span := range [][2]int{{-7, -5}, {-5, -2}, {-2, 2}, {1, 4}, {3, 6}} {
		start, end := month(span[0]), month(span[1]).AddDate(0, 0, -1)
		dates = append(dates, model.ISO50001ClauseDate{ProjectID: p.ID, ClauseID: fmt.Sprint(5 + i), StartDate: &start, EndDate: &end})
	}
	if err := repo.ReplaceClauseDates(ctx, sc, p.ID, dates); err != nil {
		return fmt.Errorf("seed demo iso50001 dates: %w", err)
	}
	notes, err := repo.Notes(ctx, sc, p.ID, nil)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		for _, n := range []struct{ clause, title, body string }{
			{"5.1", "Üst yönetim taahhüdü", "Genel müdür EnYS taahhüt yazısını imzaladı; enerji yöneticisi atandı."},
			{"5.2", "Enerji politikası", "Enerji politikası yayımlandı, tüm çalışanlara duyuruldu ve panolara asıldı."},
			{"6.3", "Enerji gözden geçirme", "Önemli enerji kullanımları belirlendi; kompresör dairesi öncelikli. Hedef: yıllık %8 tasarruf."},
			{"7.2", "Yetkinlik eğitimleri", "Bakım ekibine verimli motor ve kompanzasyon eğitimi planlandı."},
		} {
			if _, err := repo.CreateNote(ctx, sc, model.ISO50001Note{ProjectID: p.ID, ClauseID: n.clause, Title: ptr(n.title), Body: n.body,
				CreatedBy: ptr(DemoUserID)}); err != nil {
				return fmt.Errorf("seed demo iso50001 note: %w", err)
			}
		}
	}
	files := postgres.NewFileRepository(d.Pool)
	owner := "iso50001"
	have, err := files.List(ctx, sc, store.FileFilter{OwnerType: &owner, OwnerID: &factory, Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	for _, doc := range demoDocuments {
		body, err := demoPDF(doc.title, doc.lines, now)
		if err != nil {
			return err
		}
		saved, err := d.Storage.Save(ctx, b.company, owner, doc.name, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("seed demo document %s: %w", doc.name, err)
		}
		if _, err := files.Create(ctx, sc, model.StoredFile{CompanyID: b.company, OwnerType: owner, OwnerID: &factory, ClauseID: ptr(doc.clause),
			OriginalName: doc.name, StoredPath: saved.Path, ContentType: saved.ContentType, SizeBytes: saved.Size, Checksum: saved.Checksum,
			UploadedBy: ptr(DemoUserID), CreatedAt: now}); err != nil {
			return fmt.Errorf("seed demo document %s: %w", doc.name, err)
		}
	}
	return nil
}

// demoPDF renders a one-page document in the Go fonts, which carry the
// Turkish letters the core PDF fonts lack.
func demoPDF(title string, lines []string, now time.Time) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCreationDate(now)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	pdf.AddPage()
	pdf.SetFont("Go", "B", 16)
	pdf.CellFormat(0, 12, "Demo Enerji A.Ş. — "+title, "", 1, "L", false, 0, "")
	pdf.SetFont("Go", "", 11)
	pdf.CellFormat(0, 8, "Ostim Fabrika · "+now.In(istanbul).Format("02.01.2006"), "", 1, "L", false, 0, "")
	pdf.Ln(4)
	for _, l := range lines {
		pdf.MultiCell(0, 7, "• "+l, "", "L", false)
	}
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// demoHeating is the share of a 9000 m³ month the boiler burns, Jan..Dec.
var demoHeating = [12]float64{1.6, 1.4, 1.0, 0.6, 0.25, 0.1, 0.05, 0.05, 0.15, 0.5, 1.0, 1.5}

// demoActivities are the factory's monthly manual EKOCM records.
var demoActivities = []struct {
	sub, factor string
	quantity    func(time.Month) float64
	note        string
}{
	{"sub_space_heating", "stationary_space_heating_natural_gas", func(m time.Month) float64 { return 9000 * demoHeating[m-1] }, "Kazan dairesi doğalgaz faturası"},
	{"sub_other_combustion", "stationary_other_combustion_diesel", func(time.Month) float64 { return 650 }, "Forklift motorin alımı"},
	{"sub_passenger_transport", "mobility_car_diesel", func(time.Month) float64 { return 1200 }, "Şirket araçları yakıt kartı"},
	{"sub_business_travel", "business_travel_domestic_air", func(m time.Month) float64 { return 4000 + 600*float64(m%4) }, "Yurtiçi uçuşlar"},
	{"sub_employee_commuting", "employee_commuting_personnel_service", func(time.Month) float64 { return 118800 }, "Personel servisleri"}, //nolint:misspell // Turkish
}

// ensureDemoCarbon declares each building's activities, writes the factory's
// manual records, the meter-derived records and one GHG report over the last
// 12 closed months.
func ensureDemoCarbon(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewCarbonRepository(d.Pool)
	svc := carbonsvc.New(carbonsvc.Deps{Carbon: repo, Buildings: postgres.NewBuildingRepository(d.Pool),
		Analyzers: postgres.NewAnalyzerRepository(d.Pool), Analytics: postgres.NewAnalyticsRepository(d.Pool),
		Companies: postgres.NewCompanyRepository(d.Pool), Ops: postgres.NewOpsRepository(d.Pool),
		Tenants: admin.NewTenantRepository(d.Pool), Clock: clock.NewFake(now)})
	for i, keys := range [][]string{
		{"sub_space_heating", "sub_other_combustion", "sub_passenger_transport", carbondomain.SubGrid, "sub_business_travel", "sub_employee_commuting"},
		{carbondomain.SubGrid, "sub_business_travel", "sub_employee_commuting"},
		{carbondomain.SubGrid, carbondomain.SubGeneration},
	} {
		if err := repo.ReplaceSelectedActivities(ctx, sc, b.buildings[i].id, keys); err != nil {
			return fmt.Errorf("seed demo carbon selection: %w", err)
		}
	}
	local := now.In(istanbul)
	thisMonth := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := ensureDemoManualCarbon(ctx, repo, b, thisMonth, now); err != nil {
		return err
	}
	grid, err := svc.GridFactor(ctx, sc)
	if err != nil {
		return err
	}
	if err := ensureDemoAutomatedCarbon(ctx, d, b, grid, thisMonth, now); err != nil {
		return err
	}
	have, err := repo.ListReports(ctx, sc, nil, store.Page{Limit: 1})
	if err != nil || len(have) > 0 {
		return err
	}
	if _, err := svc.CreateReport(ctx, sc, carbonsvc.ReportInput{BuildingID: b.buildings[0].id, Type: "ghg",
		From: thisMonth.AddDate(-1, 0, 0), To: thisMonth.AddDate(0, 0, -1), Name: ptr("GHG Protocol — son 12 ay")}); err != nil {
		return fmt.Errorf("seed demo carbon report: %w", err)
	}
	return nil
}

// ensureDemoManualCarbon writes 12 months of the factory's manual records;
// last month's travel records still wait for approval.
func ensureDemoManualCarbon(ctx context.Context, repo *postgres.CarbonRepository, b demoBaseline, thisMonth, now time.Time) error {
	sc := store.SystemScope(b.company)
	manual, factory := false, b.buildings[0].id
	have, err := repo.ListActivities(ctx, sc, store.CarbonActivityFilter{BuildingIDs: []uuid.UUID{factory}, IsAutomated: &manual,
		Page: store.Page{Limit: 1}})
	if err != nil || len(have) > 0 {
		return err
	}
	keys := make([]string, len(demoActivities))
	for i, x := range demoActivities {
		keys[i] = x.factor
	}
	factors, err := repo.ListFactors(ctx, sc, store.EmissionFactorFilter{Keys: keys, IncludePlatform: true, Page: store.Page{Limit: 100}})
	if err != nil {
		return err
	}
	byKey := map[string]model.EmissionFactor{}
	for _, f := range factors {
		byKey[f.Key] = f
	}
	one := decimal.NewFromInt(1)
	for m := 12; m >= 1; m-- {
		start := thisMonth.AddDate(0, -m, 0)
		for i, x := range demoActivities {
			f, ok := byKey[x.factor]
			if !ok {
				continue // a catalogue without this factor shows fewer records
			}
			sub, _ := carbondomain.SubByKey(x.sub)
			q := decimal.NewFromFloat(x.quantity(start.Month())).Round(2)
			a, err := repo.CreateActivity(ctx, sc, model.CarbonActivity{CompanyID: b.company, BuildingID: factory, MainCategory: sub.Main,
				SubCategory: sub.Key, ActivityType: sub.Key, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, -1), Quantity: q,
				Unit: f.BaseUnit, FactorID: &f.ID, FactorKey: &f.Key, FactorValue: &f.BaseFactor, ConversionMultiplier: one,
				EmissionKgco2e: carbondomain.Emission(q, one, f.BaseFactor), Scope: sub.Scope, IsoCategory: sub.ISO,
				Description: ptr(x.note), Details: []byte(`{}`), Status: model.CarbonStatusPending})
			if err != nil {
				return fmt.Errorf("seed demo carbon activity %s: %w", x.sub, err)
			}
			if m == 1 && i >= 3 {
				continue
			}
			if _, err := repo.SetActivityStatus(ctx, sc, a.ID, model.CarbonStatusApproved, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureDemoAutomatedCarbon writes the rows carbon.daily_accrual derives from
// the meters: one per closed month, one per day of this month. The upsert key
// makes a rerun rewrite them in place.
func ensureDemoAutomatedCarbon(ctx context.Context, d DemoDeps, b demoBaseline, grid *carbonsvc.FactorView, thisMonth, now time.Time) error {
	if grid == nil {
		return nil
	}
	sc := store.SystemScope(b.company)
	repo, analytics := postgres.NewCarbonRepository(d.Pool), postgres.NewAnalyticsRepository(d.Pool)
	one := decimal.NewFromInt(1)
	snapshot := map[string]string{}
	if grid.Source != nil {
		snapshot["factor_source"] = *grid.Source
	}
	if grid.SourceYear != nil {
		snapshot["factor_source_year"] = fmt.Sprint(*grid.SourceYear)
	}
	type period struct{ start, end time.Time }
	for bi, x := range b.buildings {
		var ids []uuid.UUID
		for _, a := range b.analyzers {
			if a.building == bi {
				ids = append(ids, a.id)
			}
		}
		from := time.Date(thisMonth.Year(), thisMonth.Month()-12, 1, 0, 0, 0, 0, istanbul)
		buckets, err := analytics.ConsumptionDaily(ctx, sc, ids, store.TimeRange{From: from, To: istanbulDay(now)})
		if err != nil {
			return err
		}
		var order []period
		consumption, generation := map[period]decimal.Decimal{}, map[period]decimal.Decimal{}
		for _, bk := range buckets {
			l := bk.Bucket.In(istanbul)
			k := period{time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, time.UTC), time.Date(l.Year(), l.Month()+1, 0, 0, 0, 0, 0, time.UTC)}
			if !k.start.Before(thisMonth) {
				day := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
				k = period{day, day}
			}
			if _, ok := consumption[k]; !ok {
				order = append(order, k)
			}
			if bk.ActiveConsumption != nil {
				consumption[k] = consumption[k].Add(*bk.ActiveConsumption)
			} else {
				consumption[k] = consumption[k].Add(decimal.Zero)
			}
			if bk.ActiveGeneration != nil {
				generation[k] = generation[k].Add(*bk.ActiveGeneration)
			}
		}
		upsert := func(subKey string, k period, kwh, emission decimal.Decimal, desc string, extra map[string]string) error {
			sub, _ := carbondomain.SubByKey(subKey)
			details := map[string]string{}
			for _, src := range []map[string]string{snapshot, extra} {
				for kk, v := range src {
					details[kk] = v
				}
			}
			raw, err := json.Marshal(details)
			if err != nil {
				return err
			}
			_, err = repo.UpsertAutomatedActivity(ctx, sc, model.CarbonActivity{CompanyID: b.company, BuildingID: x.id, MainCategory: sub.Main,
				SubCategory: sub.Key, ActivityType: sub.Key, PeriodStart: k.start, PeriodEnd: k.end, Quantity: kwh.Round(6), Unit: "kWh",
				FactorID: &grid.ID, FactorKey: &grid.Key, FactorValue: &grid.BaseFactor, ConversionMultiplier: one, EmissionKgco2e: emission,
				Scope: sub.Scope, IsoCategory: sub.ISO, Description: &desc, Details: raw, Status: model.CarbonStatusApproved, IsAutomated: true})
			return err
		}
		for _, k := range order {
			kwh := consumption[k]
			if err := upsert(carbondomain.SubGrid, k, kwh, carbondomain.Emission(kwh, one, grid.BaseFactor), "Şebeke tüketimi (otomatik)", nil); err != nil {
				return fmt.Errorf("seed demo carbon grid: %w", err)
			}
			if gen := generation[k]; gen.IsPositive() {
				extra := map[string]string{"source": "meter_export", "avoided_kgco2e": carbondomain.Emission(gen, one, grid.BaseFactor).String()}
				// Q-F3: rooftop export is renewable, so it carries no emission itself.
				if err := upsert(carbondomain.SubGeneration, k, gen, decimal.Zero, "Üretim, sayaç ihracatı (otomatik)", extra); err != nil {
					return fmt.Errorf("seed demo carbon generation: %w", err)
				}
			}
		}
	}
	return nil
}
