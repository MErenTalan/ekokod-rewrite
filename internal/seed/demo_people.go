package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// ensureDemoPeople writes the demo user, four colleagues, the calendar, the
// integration credentials and SMTP settings. The password is set only when the
// demo user is created or when password is not empty.
func ensureDemoPeople(ctx context.Context, d DemoDeps, b demoBaseline, password string, now time.Time) error {
	if err := ensureDemoUser(ctx, d, b, password, now); err != nil {
		return err
	}
	if err := ensureDemoColleagues(ctx, d, b, now); err != nil {
		return err
	}
	if err := ensureDemoCalendar(ctx, d, b, now); err != nil {
		return err
	}
	return ensureDemoIntegrations(ctx, d, b, now)
}

func ensureDemoUser(ctx context.Context, d DemoDeps, b demoBaseline, password string, now time.Time) error {
	sc := store.SystemScope(b.company)
	users := postgres.NewUserRepository(d.Pool)
	user := model.User{ID: DemoUserID, CompanyID: b.company, Name: "Demo Kullanıcı", Email: DemoEmail, Role: model.UserRoleDemo,
		IsActive: true, Locale: "tr", CreatedAt: now, UpdatedAt: now}
	existing, err := users.Get(ctx, sc, DemoUserID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	exists := err == nil
	if !exists && password == "" {
		return errors.New("seed demo: a password is required to create the demo user")
	}
	var hash string
	if password != "" {
		in := auth.PolicyInput{Password: password, Name: user.Name, Email: DemoEmail, CompanyName: "Demo Enerji A.Ş."}
		if exists {
			in.CurrentHash = existing.PasswordHash
		}
		if codes := auth.CheckPolicy(d.Hasher, in); len(codes) > 0 {
			return fmt.Errorf("%w: %v", ErrWeakPassword, codes)
		}
		if hash, err = d.Hasher.Hash(password); err != nil {
			return err
		}
	}
	if !exists {
		user.PasswordHash = hash
		_, err := users.Create(ctx, sc, user)
		return err
	}
	// The UI preferences are the visitors' own; everything else is baseline.
	user.UIPreferences = existing.UIPreferences
	if _, err := users.Update(ctx, sc, user); err != nil {
		return err
	}
	if hash != "" {
		return users.SetPassword(ctx, sc, DemoUserID, hash, now)
	}
	return nil
}

// ensureDemoColleagues gives each colleague a random password nobody knows;
// R543 refuses their logins anyway.
func ensureDemoColleagues(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	users, buildings := postgres.NewUserRepository(d.Pool), postgres.NewBuildingRepository(d.Pool)
	for _, c := range b.colleagues {
		user := model.User{ID: c.id, CompanyID: b.company, Name: c.name, Email: c.email, Role: c.role, IsActive: true,
			Locale: "tr", CreatedAt: now, UpdatedAt: now}
		switch _, err := users.Get(ctx, sc, c.id); {
		case errors.Is(err, store.ErrNotFound):
			secret := make([]byte, 24)
			if _, err := rand.Read(secret); err != nil {
				return err
			}
			if user.PasswordHash, err = d.Hasher.Hash(hex.EncodeToString(secret)); err != nil {
				return err
			}
			if _, err := users.Create(ctx, sc, user); err != nil {
				return fmt.Errorf("seed demo colleague %s: %w", c.name, err)
			}
		case err != nil:
			return err
		default:
			if _, err := users.Update(ctx, sc, user); err != nil {
				return fmt.Errorf("seed demo colleague %s: %w", c.name, err)
			}
		}
	}
	for i, x := range b.buildings {
		row, err := buildings.Get(ctx, sc, x.id)
		if err != nil {
			return err
		}
		row.ResponsibleUserID = nil
		for _, c := range b.colleagues {
			if c.building == i {
				row.ResponsibleUserID = &c.id
			}
		}
		if _, err := buildings.Update(ctx, sc, row); err != nil {
			return fmt.Errorf("seed demo building owner %s: %w", x.name, err)
		}
	}
	return nil
}

// demoHolidays are the Turkish public holidays, inclusive ranges; the 2027
// religious holidays follow the Diyanet calendar.
var demoHolidays = []struct {
	from, to string
	name     string
}{
	{"2025-01-01", "2025-01-01", "Yılbaşı"}, {"2025-03-30", "2025-04-01", "Ramazan Bayramı"},
	{"2025-04-23", "2025-04-23", "Ulusal Egemenlik ve Çocuk Bayramı"}, {"2025-05-01", "2025-05-01", "Emek ve Dayanışma Günü"},
	{"2025-05-19", "2025-05-19", "Atatürk'ü Anma, Gençlik ve Spor Bayramı"}, {"2025-06-06", "2025-06-09", "Kurban Bayramı"},
	{"2025-07-15", "2025-07-15", "Demokrasi ve Millî Birlik Günü"}, {"2025-08-30", "2025-08-30", "Zafer Bayramı"},
	{"2025-10-29", "2025-10-29", "Cumhuriyet Bayramı"},
	{"2026-01-01", "2026-01-01", "Yılbaşı"}, {"2026-03-20", "2026-03-22", "Ramazan Bayramı"},
	{"2026-04-23", "2026-04-23", "Ulusal Egemenlik ve Çocuk Bayramı"}, {"2026-05-01", "2026-05-01", "Emek ve Dayanışma Günü"},
	{"2026-05-19", "2026-05-19", "Atatürk'ü Anma, Gençlik ve Spor Bayramı"}, {"2026-05-27", "2026-05-30", "Kurban Bayramı"},
	{"2026-07-15", "2026-07-15", "Demokrasi ve Millî Birlik Günü"}, {"2026-08-30", "2026-08-30", "Zafer Bayramı"},
	{"2026-10-29", "2026-10-29", "Cumhuriyet Bayramı"},
	{"2027-01-01", "2027-01-01", "Yılbaşı"}, {"2027-03-09", "2027-03-11", "Ramazan Bayramı"},
	{"2027-04-23", "2027-04-23", "Ulusal Egemenlik ve Çocuk Bayramı"}, {"2027-05-01", "2027-05-01", "Emek ve Dayanışma Günü"},
	{"2027-05-16", "2027-05-19", "Kurban Bayramı"}, {"2027-05-19", "2027-05-19", "Atatürk'ü Anma, Gençlik ve Spor Bayramı"},
	{"2027-07-15", "2027-07-15", "Demokrasi ve Millî Birlik Günü"}, {"2027-08-30", "2027-08-30", "Zafer Bayramı"},
	{"2027-10-29", "2027-10-29", "Cumhuriyet Bayramı"},
}

func demoHoliday(local time.Time) bool {
	day := local.Format(time.DateOnly)
	for _, h := range demoHolidays {
		if day >= h.from && day <= h.to {
			return true
		}
	}
	return false
}

func ensureDemoCalendar(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	calendar := postgres.NewCalendarRepository(d.Pool)
	if err := calendar.ReplaceWeekendDays(ctx, sc, []int16{0, 6}); err != nil {
		return fmt.Errorf("seed demo weekend days: %w", err)
	}
	have, err := calendar.Vacations(ctx, sc, nil)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, v := range have {
		seen[v.StartDate.Format(time.DateOnly)+v.EndDate.Format(time.DateOnly)] = true
	}
	for _, h := range demoHolidays {
		if seen[h.from+h.to] {
			continue
		}
		from, _ := time.ParseInLocation(time.DateOnly, h.from, istanbul)
		to, _ := time.ParseInLocation(time.DateOnly, h.to, istanbul)
		if _, err := calendar.CreateVacation(ctx, sc, model.CompanyVacation{CompanyID: b.company, StartDate: from, EndDate: to,
			Description: ptr(h.name)}); err != nil {
			return fmt.Errorf("seed demo vacation: %w", err)
		}
	}
	return ensureDemoEvents(ctx, calendar, b, now)
}

// ensureDemoEvents adds the factory's planned maintenance on the second
// Saturday of each month around now and an internal audit in three weeks.
func ensureDemoEvents(ctx context.Context, calendar *postgres.CalendarRepository, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	local := now.In(istanbul)
	type event struct {
		title      string
		start, end time.Time
		colour     string
	}
	var want []event
	for m := -2; m <= 3; m++ {
		first := time.Date(local.Year(), local.Month()+time.Month(m), 1, 8, 0, 0, 0, istanbul)
		sat := first.AddDate(0, 0, (int(time.Saturday)-int(first.Weekday())+7)%7+7)
		want = append(want, event{"Planlı bakım — Ostim Fabrika", sat, sat.Add(8 * time.Hour), "#f59e0b"})
	}
	audit := time.Date(local.Year(), local.Month(), local.Day()+21, 9, 30, 0, 0, istanbul)
	want = append(want, event{"ISO 50001 iç denetimi", audit, audit.Add(7 * time.Hour), "#2563eb"})
	have, err := calendar.ListEvents(ctx, sc, store.CalendarFilter{Page: store.Page{Limit: 500}})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, e := range have {
		seen[e.Title+e.StartsAt.UTC().String()] = true
	}
	for _, e := range want {
		if seen[e.title+e.start.UTC().String()] {
			continue
		}
		if _, err := calendar.CreateEvent(ctx, sc, model.CalendarEvent{CompanyID: b.company, Title: e.title, StartsAt: e.start.UTC(),
			EndsAt: e.end.UTC(), Colour: ptr(e.colour), CreatedBy: ptr(DemoUserID), CreatedAt: now}); err != nil {
			return fmt.Errorf("seed demo event: %w", err)
		}
	}
	return nil
}

// ensureDemoIntegrations seeds verified credentials for the catalogue
// definitions this platform has; the demo never creates a definition, so a
// provider without one is skipped (R545: none of them is ever called).
func ensureDemoIntegrations(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	repo := postgres.NewIntegrationRepository(d.Pool, d.Cipher)
	defs, err := repo.Definitions(ctx, sc)
	if err != nil {
		return err
	}
	var isolarCred *model.IntegrationCredential
	for _, want := range []struct {
		provider       model.IntegrationProvider
		subtype        string // empty: the first definition of the provider
		username       string
		secret, extra  []byte
		pm5340, region *string
	}{
		{provider: model.IntegrationProviderOSOS, subtype: "Baskent", username: "demo.osos", secret: []byte("demo-secret")},
		{provider: model.IntegrationProviderGridbox, username: "demo.gridbox", secret: []byte("demo-secret")},
		{provider: model.IntegrationProviderISolar, subtype: "EU", region: ptr("EU"),
			extra: []byte(`{"access_token":"demo-access","refresh_token":"demo-refresh"}`)},
		{provider: model.IntegrationProviderPM5340, pm5340: ptr("http://pm5340.demo.invalid"), username: "demo", secret: []byte("demo-secret")},
	} {
		var def *model.IntegrationDefinition
		for i := range defs {
			if defs[i].Provider == want.provider && (want.subtype == "" || defs[i].Subtype == want.subtype) {
				def = &defs[i]
				break
			}
		}
		if def == nil {
			continue
		}
		var username *string
		if want.username != "" {
			username = ptr(want.username)
		}
		saved, err := repo.UpsertCredential(ctx, sc, model.IntegrationCredential{ID: demoID("credential/" + string(want.provider)),
			CompanyID: b.company, DefinitionID: def.ID, Username: username, Settings: json.RawMessage(`{}`), Pm5340URL: want.pm5340,
			IsolarRegion: want.region, IsActive: true}, want.secret, want.extra)
		if err != nil {
			return fmt.Errorf("seed demo credential %s: %w", want.provider, err)
		}
		var expires *time.Time
		if want.provider == model.IntegrationProviderISolar {
			expires = ptr(now.Add(30 * 24 * time.Hour).UTC())
			isolarCred = &saved
		}
		if err := repo.RecordVerification(ctx, sc, saved.ID, now.Add(-2*time.Hour).UTC(), expires); err != nil {
			return err
		}
	}
	if err := ensureDemoIsolarLink(ctx, d, b, isolarCred, now); err != nil {
		return err
	}
	_, err = postgres.NewSMTPRepository(d.Pool, d.Cipher).Upsert(ctx, sc, model.SMTPSettings{CompanyID: b.company,
		Host: "smtp.demo.invalid", Port: 587, Username: "bildirim@demo-enerji.example", FromAddress: "bildirim@demo-enerji.example",
		UpdatedAt: now}, []byte("demo-smtp-password"))
	return err
}

// ensureDemoIsolarLink links the field plant to the seeded iSolar credential,
// marked synced 20 minutes ago with whole history through yesterday.
func ensureDemoIsolarLink(ctx context.Context, d DemoDeps, b demoBaseline, cred *model.IntegrationCredential, now time.Time) error {
	if cred == nil {
		return nil
	}
	sc := store.SystemScope(b.company)
	plants := postgres.NewPlantRepository(d.Pool)
	for _, x := range b.plants {
		if !x.isolar {
			continue
		}
		p, err := plants.Get(ctx, sc, x.id)
		if err != nil {
			return err
		}
		installed := decimal.NewFromInt(x.kwp)
		p.IsolarPsID, p.IsolarPsName, p.IsolarInstalledKw = ptr(isolar.DemoPSID), ptr(x.name), &installed
		p.IsolarLinkedAt, p.IsolarCredentialID = ptr(x.installed.AddDate(0, 1, 0)), &cred.ID
		if _, err := plants.SetIsolarLink(ctx, sc, p); err != nil {
			return fmt.Errorf("seed demo isolar link: %w", err)
		}
		if err := plants.SetSyncState(ctx, sc, x.id, now.Add(-20*time.Minute).UTC(), nil); err != nil {
			return err
		}
		if err := plants.SetHistoryThrough(ctx, sc, x.id, istanbulDay(now).AddDate(0, 0, -1)); err != nil {
			return err
		}
	}
	return nil
}
