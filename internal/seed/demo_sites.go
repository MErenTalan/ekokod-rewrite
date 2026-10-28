package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// DemoDeps is what the demo generator writes with.
type DemoDeps struct {
	Pool    *pgxpool.Pool
	Hasher  auth.Hasher
	Cipher  *crypto.Cipher // integration secrets and the SMTP password
	Billing *billingsvc.Service
	Log     *slog.Logger
}

type demoBuilding struct {
	id                         uuid.UUID
	name, address, lat, lon    string
	area                       string
	people                     int32
	cutoff                     int16
	province, district, sector string
}

// demoAnalyzer's day/night are kWh per working and idle hour; the shares are
// per-mille ranges of the active step (R550).
type demoAnalyzer struct {
	id                  uuid.UUID
	number, name        string
	building            int
	powerKw             string
	day, night          int64
	from, to            int     // local working hours [from, to)
	weekend             float64 // load factor on weekends and holidays
	season              float64 // +: summer peak, -: winter peak
	inductive           [2]int64
	summerInductive     [2]int64 // a failing compensation bank in Jun–Aug; zero = same as inductive
	solarPlant          int      // index of the netted rooftop plant, -1 = none
	latitude, longitude string
}

type demoPlant struct {
	id                  uuid.UUID
	name, kind, address string
	lat, lon            float64
	kwp                 int64
	inverters           int
	inverterModel       string
	netting             int // analyzer index, -1 = none
	isolar              bool
	installed           time.Time
}

type demoColleague struct {
	id          uuid.UUID
	name, email string
	role        model.UserRole
	building    int // the building this user is responsible for, -1 = none
}

type demoBaseline struct {
	company    uuid.UUID
	buildings  []demoBuilding
	analyzers  []demoAnalyzer
	plants     []demoPlant
	colleagues []demoColleague
}

func demoID(name string) uuid.UUID { return uuid.NewSHA1(demoNamespace, []byte(name)) }

// DemoBaseline is the demo company's fixed shape; values are relative to
// nothing, so two calls are equal (R550).
func DemoBaseline() demoBaseline {
	return demoBaseline{
		company: DemoCompanyID,
		buildings: []demoBuilding{
			{id: DemoBuildingID, name: "Ostim Fabrika", address: "Ostim OSB 1234. Cadde No: 5, Yenimahalle", lat: "39.971400", lon: "32.743400",
				area: "12000.00", people: 180, cutoff: 1, province: "Ankara", district: "Yenimahalle", sector: "Üretim"},
			{id: demoID("building/office"), name: "Merkez Ofis", address: "Kızılırmak Mahallesi 1450. Sokak No: 10, Çankaya", lat: "39.908700", lon: "32.859700",
				area: "1500.00", people: 45, cutoff: 1, province: "Ankara", district: "Çankaya", sector: "Ofis"},
			{id: demoID("building/warehouse"), name: "Gebze Lojistik Depo", address: "Gebze OSB 3800. Sokak No: 12, Gebze", lat: "40.802800", lon: "29.430700",
				area: "5000.00", people: 15, cutoff: 1, province: "Kocaeli", district: "Gebze", sector: "Lojistik"},
		},
		analyzers: []demoAnalyzer{
			{id: demoID("analyzer/v2/1"), number: "DEMO-1001", name: "Fabrika ana trafo", building: 0, powerKw: "400.000", day: 160, night: 70,
				from: 6, to: 22, weekend: 0.45, season: -0.06, inductive: [2]int64{60, 140}, solarPlant: -1, latitude: "39.971400", longitude: "32.743400"},
			{id: demoID("analyzer/v2/2"), number: "DEMO-1002", name: "Üretim hattı", building: 0, powerKw: "150.000", day: 90, night: 20,
				from: 6, to: 22, weekend: 0.3, season: 0.02, inductive: [2]int64{70, 150}, solarPlant: -1, latitude: "39.971500", longitude: "32.743600"},
			{id: demoID("analyzer/v2/3"), number: "DEMO-1003", name: "Kompresör dairesi", building: 0, powerKw: "75.000", day: 45, night: 30,
				from: 6, to: 22, weekend: 0.6, season: 0.08, inductive: [2]int64{100, 160}, summerInductive: [2]int64{260, 340}, solarPlant: -1,
				latitude: "39.971200", longitude: "32.743200"},
			{id: demoID("analyzer/v2/4"), number: "DEMO-2001", name: "Ofis ana sayaç", building: 1, powerKw: "60.000", day: 35, night: 6,
				from: 8, to: 19, weekend: 0.25, season: 0.18, inductive: [2]int64{50, 110}, solarPlant: -1, latitude: "39.908700", longitude: "32.859700"},
			{id: demoID("analyzer/v2/5"), number: "DEMO-3001", name: "Depo ana sayaç", building: 2, powerKw: "120.000", day: 40, night: 12,
				from: 7, to: 19, weekend: 0.4, season: 0.05, inductive: [2]int64{60, 120}, solarPlant: 0, latitude: "40.802800", longitude: "29.430700"},
			{id: demoID("analyzer/v2/6"), number: "DEMO-3002", name: "Soğuk oda", building: 2, powerKw: "30.000", day: 18, night: 16,
				from: 0, to: 24, weekend: 1, season: 0.25, inductive: [2]int64{80, 140}, solarPlant: -1, latitude: "40.802900", longitude: "29.430900"},
		},
		plants: []demoPlant{
			{id: demoID("plant/gebze"), name: "Gebze Çatı GES", kind: "rooftop", address: "Gebze OSB 3800. Sokak No: 12, Gebze, Kocaeli",
				lat: 40.8028, lon: 29.4307, kwp: 250, inverters: 2, inverterModel: "SG125CX", netting: 4,
				installed: time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC)},
			{id: demoID("plant/konya"), name: "Konya Arazi GES", kind: "grid", address: "Karatay, Konya",
				lat: 37.8746, lon: 32.4932, kwp: 1000, inverters: 4, inverterModel: "SG250HX", netting: -1, isolar: true,
				installed: time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)},
		},
		colleagues: []demoColleague{
			{id: demoID("user/ayse"), name: "Ayşe Yılmaz", email: "ayse.yilmaz@demo-enerji.example", role: model.UserRoleBuildingAdmin, building: 0},
			{id: demoID("user/mehmet"), name: "Mehmet Kaya", email: "mehmet.kaya@demo-enerji.example", role: model.UserRoleCompanyReadonlyAdmin, building: -1},
			{id: demoID("user/zeynep"), name: "Zeynep Demir", email: "zeynep.demir@demo-enerji.example", role: model.UserRoleBuildingReadonlyAdmin, building: 1},
			{id: demoID("user/can"), name: "Can Öztürk", email: "can.ozturk@demo-enerji.example", role: model.UserRoleBuildingAdmin, building: 2},
		},
	}
}

// Keep is what a reset must not delete (R546).
func (b demoBaseline) Keep() admin.DemoKeep {
	k := admin.DemoKeep{User: DemoUserID}
	for _, x := range b.buildings {
		k.Buildings = append(k.Buildings, x.id)
	}
	for _, x := range b.analyzers {
		k.Analyzers = append(k.Analyzers, x.id)
	}
	for _, x := range b.plants {
		k.Plants = append(k.Plants, x.id)
	}
	return k
}

func ptr[T any](v T) *T { return &v }

// ensureDemoSites creates the company, buildings, analyzers, plants and
// inverters, or writes them back to baseline when a visitor changed them.
func ensureDemoSites(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	if err := admin.NewDemoRepository(d.Pool).Restore(ctx, b.company, b.Keep()); err != nil {
		return err
	}
	dec := decimal.RequireFromString
	company := model.Company{ID: b.company, Name: "Demo Enerji A.Ş.", Address: ptr("Ostim OSB, Yenimahalle, Ankara"),
		TotalAreaM2: ptr(dec("18500.00")), PersonnelCount: ptr(int32(240)), ContactName: ptr("Demo Kullanıcı"),
		ContactPhone: ptr("+90 312 000 00 00"), Sector: ptr("Üretim"), CreatedAt: now, UpdatedAt: now}
	companies := postgres.NewCompanyRepository(d.Pool)
	if err := upsert(ctx, companies.Get, companies.Create, companies.Update, sc, b.company, company); err != nil {
		return fmt.Errorf("seed demo company: %w", err)
	}
	// R540: the tag is what every guard reads; an older demo row gets it too.
	if err := admin.NewTenantRepository(d.Pool).SetDemo(ctx, b.company, true); err != nil {
		return fmt.Errorf("seed demo tag: %w", err)
	}
	buildings := postgres.NewBuildingRepository(d.Pool)
	for _, x := range b.buildings {
		lat, lon := dec(x.lat), dec(x.lon)
		row := model.Building{ID: x.id, CompanyID: b.company, Name: x.name, Address: ptr(x.address), Latitude: &lat, Longitude: &lon,
			PersonnelCount: ptr(x.people), TotalAreaM2: ptr(dec(x.area)), Sector: ptr(x.sector), BillCutoffDay: x.cutoff,
			CreatedAt: now, UpdatedAt: now}
		if err := upsert(ctx, buildings.Get, buildings.Create, buildings.Update, sc, x.id, row); err != nil {
			return fmt.Errorf("seed demo building %s: %w", x.name, err)
		}
	}
	analyzers := postgres.NewAnalyzerRepository(d.Pool)
	for _, x := range b.analyzers {
		building, lat, lon := b.buildings[x.building], dec(x.latitude), dec(x.longitude)
		row := model.Analyzer{ID: x.id, CompanyID: b.company, BuildingID: &building.id, Provider: model.IntegrationProviderOSOS,
			ProviderSubtype: "Baskent", InstallationNumber: x.number, CustomerName: ptr("Demo Enerji A.Ş."), Address: ptr(building.address),
			Province: ptr(building.province), District: ptr(building.district), InstalledPowerKw: ptr(dec(x.powerKw)),
			MeterNumber: ptr("7" + x.number[5:] + "0042"), MeterModel: ptr("Makel C510"), MeterMultiplier: decimal.NewFromInt(1),
			MeteringPointName: ptr(x.name), Latitude: &lat, Longitude: &lon, IsActive: true, CreatedAt: now, UpdatedAt: now}
		if err := upsert(ctx, analyzers.Get, analyzers.Create, analyzers.Update, sc, x.id, row); err != nil {
			return fmt.Errorf("seed demo analyzer %s: %w", x.number, err)
		}
	}
	return ensureDemoPlants(ctx, d, b, now)
}

// upsert overwrites row when it exists and creates it otherwise.
func upsert[T any](ctx context.Context, get func(context.Context, store.Scope, uuid.UUID) (T, error),
	create, update func(context.Context, store.Scope, T) (T, error), sc store.Scope, id uuid.UUID, row T) error {
	_, err := get(ctx, sc, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		_, err = create(ctx, sc, row)
	case err == nil:
		_, err = update(ctx, sc, row)
	}
	return err
}

func ensureDemoPlants(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) error {
	sc := store.SystemScope(b.company)
	plants := postgres.NewPlantRepository(d.Pool)
	for _, x := range b.plants {
		lat, lon := decimal.NewFromFloat(x.lat).Round(6), decimal.NewFromFloat(x.lon).Round(6)
		kwp := decimal.NewFromInt(x.kwp)
		panels := int32(x.kwp * 1000 / 550)
		row := model.PowerPlant{ID: x.id, CompanyID: b.company, Name: x.name, InstallationNumber: ptr("GES-" + x.id.String()[:6]),
			PlantKind: x.kind, PvBrandModel: ptr("Jinko Tiger Neo 550 W"), PanelPowerW: ptr(decimal.NewFromInt(550)),
			PanelEfficiencyPct: ptr(decimal.RequireFromString("21.30")), PanelCount: &panels, StringCount: ptr(panels / 20),
			Orientation: ptr(model.PanelOrientationS), TiltAngleDeg: ptr(decimal.NewFromInt(25)), TotalCapacityKw: &kwp,
			YearlyTargetKwh: ptr(decimal.NewFromInt(x.kwp * demoSpecificYield)), InstallationDate: ptr(x.installed),
			Address: ptr(x.address), Latitude: &lat, Longitude: &lon, CreatedAt: now, UpdatedAt: now}
		if x.netting >= 0 {
			row.NettingAnalyzerID = &b.analyzers[x.netting].id
		}
		if err := upsert(ctx, plants.Get, plants.Create, plants.Update, sc, x.id, row); err != nil {
			return fmt.Errorf("seed demo plant %s: %w", x.name, err)
		}
		var targets []model.PlantMonthlyTarget
		for m, share := range demoMonthlyYield {
			targets = append(targets, model.PlantMonthlyTarget{PlantID: x.id, Month: int16(m + 1), TargetKwh: decimal.NewFromInt(x.kwp * share)})
		}
		if _, err := plants.ReplaceMonthlyTargets(ctx, sc, x.id, targets); err != nil {
			return fmt.Errorf("seed demo plant targets: %w", err)
		}
		if err := ensureDemoInverters(ctx, plants, sc, x, now); err != nil {
			return err
		}
	}
	return nil
}

// demoMonthlyYield is kWh per kWp per month; it sums to demoSpecificYield.
var demoMonthlyYield = [12]int64{62, 80, 118, 140, 165, 180, 190, 178, 145, 110, 72, 60}

const demoSpecificYield = 1500

func ensureDemoInverters(ctx context.Context, plants *postgres.PlantRepository, sc store.Scope, x demoPlant, now time.Time) error {
	local := now.In(istanbul)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, istanbul)
	month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, istanbul)
	year := time.Date(local.Year(), 1, 1, 0, 0, 0, 0, istanbul)
	end := now.Truncate(time.Hour)
	share := decimal.NewFromInt(int64(x.inverters))
	sum := func(from time.Time) decimal.Decimal {
		total := 0.0
		for ts := from; ts.Before(end); ts = ts.Add(time.Hour) {
			total += solarKwh(x, ts)
		}
		return decimal.NewFromFloat(total).Div(share).Round(1)
	}
	today, monthly, yearly := sum(day), sum(month), sum(year)
	prior := decimal.NewFromInt(x.kwp * demoSpecificYield * int64(local.Year()-x.installed.Year()) / int64(x.inverters))
	power := decimal.NewFromFloat(solarKwh(x, end)).Div(share).Round(2)
	for i := 1; i <= x.inverters; i++ {
		sn := fmt.Sprintf("%s-INV-%d", x.inverterModel, i)
		dev := model.PlantDevice{PlantID: x.id, DeviceSN: sn, DeviceName: ptr(fmt.Sprintf("İnverter %d", i)), DeviceType: ptr(int32(1)),
			DeviceTypeName: ptr("İnverter"), Brand: ptr("Sungrow"), Model: ptr(x.inverterModel),
			RatedPowerKw: ptr(decimal.NewFromInt(x.kwp / int64(x.inverters))), Status: ptr("normal"),
			EfficiencyPct: ptr(decimal.RequireFromString("98.400")), LastSeenAt: ptr(now.Add(-10 * time.Minute).UTC())}
		if x.isolar {
			dev.ProviderKey = ptr(fmt.Sprintf("%s_1_%d_1", isolar.DemoPSID, i))
		}
		saved, err := plants.UpsertDevice(ctx, sc, dev)
		if err != nil {
			return fmt.Errorf("seed demo inverter %s: %w", sn, err)
		}
		saved.SnapshotAt, saved.FaultStatus, saved.ActivePowerKw = ptr(now.Add(-10*time.Minute).UTC()), ptr(int32(4)), &power
		saved.YieldTodayKwh, saved.YieldMonthKwh, saved.YieldYearKwh = &today, &monthly, &yearly
		saved.YieldTotalKwh = ptr(yearly.Add(prior))
		if err := plants.UpdateDeviceSnapshot(ctx, sc, saved); err != nil {
			return fmt.Errorf("seed demo inverter snapshot %s: %w", sn, err)
		}
	}
	return nil
}
