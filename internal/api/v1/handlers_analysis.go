package v1

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/grouping"
	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/table"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/analysis"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
)

func analysisRoutes() []Route {
	all := auth.AllRoles
	return []Route{
		{Method: http.MethodGet, Pattern: "/consumption", OperationID: "consumption.series", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "Consumption series for an analyzer or a building (summed).", Request: dto.ConsumptionQuery{},
			Response: dto.ConsumptionSeries{}, Status: http.StatusOK, Handler: (*Handlers).consumptionSeries},
		{Method: http.MethodGet, Pattern: "/consumption/summary", OperationID: "consumption.summary", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "Totals, averages, peak and valley for the range.", Request: dto.ConsumptionQuery{},
			Response: dto.ConsumptionSummary{}, Status: http.StatusOK, Handler: (*Handlers).consumptionSummary},
		{Method: http.MethodGet, Pattern: "/consumption/grouped", OperationID: "consumption.grouped", Tag: "consumption",
			Access: RoleGated, Roles: all, Summary: "Consumption grouped by week, day type or season, with the previous period (R193).",
			Request: dto.ConsumptionGroupedQuery{}, Response: dto.ConsumptionGrouped{}, Status: http.StatusOK,
			Handler: (*Handlers).consumptionGrouped},
		{Method: http.MethodGet, Pattern: "/consumption/export", OperationID: "consumption.export", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "The series as CSV or XLSX.", Request: dto.ConsumptionExportQuery{}, Status: http.StatusOK,
			RawContentType: "application/octet-stream", Handler: (*Handlers).consumptionExport},
		{Method: http.MethodGet, Pattern: "/consumption/anomalies", OperationID: "consumption.anomalies", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "Suspect periods.", Request: dto.AnomalyQuery{}, Response: dto.Page[dto.Anomaly]{}, Status: http.StatusOK,
			Handler: (*Handlers).listAnomalies},
		{Method: http.MethodPost, Pattern: "/consumption/anomalies/{id}/resolve", OperationID: "consumption.anomalies.resolve", Tag: "consumption",
			Access: RoleGated, Roles: auth.Roles(roleA, roleCA), Entity: "consumption_anomaly",
			Summary: "Resolve a suspect period: register a reset, override, or accept.", Request: dto.AnomalyResolveRequest{},
			Response: dto.Anomaly{}, Status: http.StatusOK, Handler: (*Handlers).resolveAnomaly},
		{Method: http.MethodGet, Pattern: "/consumption/reactive-status", OperationID: "consumption.reactive_status", Tag: "consumption",
			Access: RoleGated, Roles: all, Summary: "Reactive ratios, limits and penalty status for a month (R166).",
			Request: dto.ReactiveStatusQuery{}, Response: dto.ReactiveStatus{}, Status: http.StatusOK, Handler: (*Handlers).reactiveStatus},
		{Method: http.MethodGet, Pattern: "/load-profile", OperationID: "load_profile.profiles", Tag: "load-profile", Access: RoleGated, Roles: all,
			Summary: "Averaged 24-hour profiles.", Request: dto.LoadProfileQuery{}, Response: dto.LoadProfiles{}, Status: http.StatusOK,
			Handler: (*Handlers).loadProfiles},
		{Method: http.MethodGet, Pattern: "/load-profile/statistics", OperationID: "load_profile.statistics", Tag: "load-profile", Access: RoleGated,
			Roles: all, Summary: "Per-profile statistics.", Request: dto.LoadProfileQuery{}, Response: dto.LoadProfileStatistics{},
			Status: http.StatusOK, Handler: (*Handlers).loadProfileStatistics},
		{Method: http.MethodGet, Pattern: "/load-profile/export", OperationID: "load_profile.export", Tag: "load-profile", Access: RoleGated,
			Roles: all, Summary: "The hourly matrix as XLSX.", Request: dto.LoadProfileQuery{}, Status: http.StatusOK,
			RawContentType: xlsxType, Handler: (*Handlers).loadProfileExport},
		{Method: http.MethodGet, Pattern: "/generation", OperationID: "generation.series", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "Generation series from the export registers.", Request: dto.ConsumptionQuery{}, Response: dto.ConsumptionSeries{},
			Status: http.StatusOK, Handler: (*Handlers).generationSeries},
		{Method: http.MethodGet, Pattern: "/energy-balance", OperationID: "energy_balance", Tag: "consumption", Access: RoleGated, Roles: all,
			Summary: "Consumption, generation, grid import and export.", Request: dto.ConsumptionQuery{}, Response: dto.EnergyBalance{},
			Status: http.StatusOK, Handler: (*Handlers).energyBalance},
		{Method: http.MethodPost, Pattern: "/anomaly/check", OperationID: "anomaly.check", Tag: "forecast", Access: RoleGated,
			Roles: auth.Roles(roleA, roleCA, roleBA), Entity: "analyzer", NoIdempotency: true,
			Summary: "Check one value against the model (unavailable until the ML service, R165).", Request: dto.AnomalyCheckRequest{},
			Response: dto.AnomalyCheck{}, Status: http.StatusOK, Handler: (*Handlers).anomalyCheck},
	}
}

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func seriesInput(q dto.ConsumptionQuery) analysis.SeriesInput {
	return analysis.SeriesInput{
		Subject: analysis.Subject{AnalyzerID: q.AnalyzerID, BuildingID: q.BuildingID},
		Level:   energy.Level(q.Granularity), From: q.From.Time, To: q.To.Time,
	}
}

func decOrNil(d *decimal.Decimal) *dto.Decimal { return dto.DP(d) }

func registersOf(values map[energy.Register]*decimal.Decimal, suspect map[energy.Register]energy.Suspicion) dto.Registers {
	get := func(r energy.Register) *dto.Decimal {
		if _, bad := suspect[r]; bad {
			return nil
		}
		return decOrNil(values[r])
	}
	return dto.Registers{
		ActiveImport: get(energy.ActiveImport), ReactiveInductiveImport: get(energy.ReactiveInductiveImport),
		ReactiveCapacitiveImport: get(energy.ReactiveCapacitiveImport), T1Import: get(energy.T1Import), T2Import: get(energy.T2Import),
		T3Import: get(energy.T3Import), ActiveExport: get(energy.ActiveExport), ReactiveInductiveExport: get(energy.ReactiveInductiveExport),
		ReactiveCapacitiveExport: get(energy.ReactiveCapacitiveExport), T1Export: get(energy.T1Export), T2Export: get(energy.T2Export),
		T3Export: get(energy.T3Export),
	}
}

func rowDTO(r consumption.Row, building bool) dto.ConsumptionRow {
	out := dto.ConsumptionRow{
		PeriodStart: dto.T(r.Window.From), PeriodEnd: dto.T(r.Window.To), Source: string(r.Source), Partial: r.Partial,
		SuspectRegisters: []string{}, Registers: registersOf(r.Values, r.Suspect),
		InductiveRatio: decOrNil(r.InductiveRatio), CapacitiveRatio: decOrNil(r.CapacitiveRatio), MaxDemandKw: decOrNil(r.MaxDemandKw),
	}
	if !building {
		id := r.AnalyzerID
		out.AnalyzerID = &id
		idx := registersOf(r.Indexes, r.Suspect)
		out.RegisterIndexes = dto.RegisterIndexes{
			ActiveImportIndex: idx.ActiveImport, ReactiveInductiveImportIndex: idx.ReactiveInductiveImport,
			ReactiveCapacitiveImportIndex: idx.ReactiveCapacitiveImport, T1ImportIndex: idx.T1Import, T2ImportIndex: idx.T2Import,
			T3ImportIndex: idx.T3Import, ActiveExportIndex: idx.ActiveExport, ReactiveInductiveExportIndex: idx.ReactiveInductiveExport,
			ReactiveCapacitiveExportIndex: idx.ReactiveCapacitiveExport, T1ExportIndex: idx.T1Export, T2ExportIndex: idx.T2Export,
			T3ExportIndex: idx.T3Export,
		}
	}
	for _, reg := range energy.AllRegisters() {
		if _, bad := r.Suspect[reg]; bad {
			out.SuspectRegisters = append(out.SuspectRegisters, string(reg))
		}
	}
	return out
}

func (h *Handlers) series(w http.ResponseWriter, r *http.Request, generation bool) {
	serve(w, r, http.StatusOK, func(q dto.ConsumptionQuery) (any, error) {
		rows, building, err := h.Analysis.Rows(r.Context(), mw.ScopeFrom(r), seriesInput(q))
		if err != nil {
			return nil, err
		}
		if generation {
			rows = consumption.GenerationRows(rows)
		}
		out := dto.ConsumptionSeries{Items: make([]dto.ConsumptionRow, len(rows))}
		for i, row := range rows {
			out.Items[i] = rowDTO(row, building)
		}
		return out, nil
	})
}

func (h *Handlers) consumptionSeries(w http.ResponseWriter, r *http.Request) { h.series(w, r, false) }
func (h *Handlers) generationSeries(w http.ResponseWriter, r *http.Request)  { h.series(w, r, true) }

func (h *Handlers) consumptionSummary(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ConsumptionQuery) (any, error) {
		rows, _, err := h.Analysis.Rows(r.Context(), mw.ScopeFrom(r), seriesInput(q))
		if err != nil {
			return nil, err
		}
		s := consumption.Summarise(rows)
		out := dto.ConsumptionSummary{Rows: s.Rows, SuspectRows: s.Suspect, Totals: registersOf(s.Totals, nil),
			Averages: registersOf(s.Averages, nil), MaxDemandKw: decOrNil(s.MaxDemandKw)}
		peak := func(row *consumption.Row) *dto.PeriodValue {
			if row == nil || row.Values[energy.ActiveImport] == nil {
				return nil
			}
			return &dto.PeriodValue{PeriodStart: dto.T(row.Window.From), ActiveImport: dto.D(*row.Values[energy.ActiveImport])}
		}
		out.Peak, out.Valley = peak(s.Peak), peak(s.Valley)
		return out, nil
	})
}

func (h *Handlers) consumptionExport(w http.ResponseWriter, r *http.Request) {
	var q dto.ConsumptionExportQuery
	if err := kit.Bind(r, &q); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	rows, _, err := h.Analysis.Rows(r.Context(), mw.ScopeFrom(r), seriesInput(q.ConsumptionQuery))
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	export := consumption.ExportRows(rows)
	name := fmt.Sprintf("tuketim-%s-%s", q.From.Format("20060102"), q.To.Format("20060102"))
	var body []byte
	contentType := "text/csv; charset=utf-8"
	if q.Format == "xlsx" {
		body, err = table.XLSX("Tuketim", export.Columns, export.Rows)
		contentType = xlsxType
	} else {
		body, err = table.CSV(export.Columns, export.Rows)
	}
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, contentType, name+"."+q.Format, body)
}

func writeFile(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func anomalyDTO(a model.ConsumptionAnomaly) dto.Anomaly {
	return dto.Anomaly{ID: a.ID, AnalyzerID: a.AnalyzerID, PeriodStart: dto.T(a.PeriodStart), PeriodEnd: dto.T(a.PeriodEnd),
		Reason: a.Reason, Detail: a.Detail, Resolution: a.Resolution, ResolvedAt: dto.TP(a.ResolvedAt), ResolvedBy: a.ResolvedBy,
		CreatedAt: dto.T(a.CreatedAt)}
}

func (h *Handlers) listAnomalies(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AnomalyQuery) (any, error) {
		page, limit, err := kit.ResolvePage(q.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Analysis.Anomalies(r.Context(), mw.ScopeFrom(r), analysis.AnomalyInput{
			Subject: analysis.Subject{AnalyzerID: q.AnalyzerID, BuildingID: q.BuildingID}, Unresolved: q.Unresolved, Page: page,
		})
		if err != nil {
			return nil, err
		}
		items := make([]dto.Anomaly, len(list))
		for i, a := range list {
			items[i] = anomalyDTO(a)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func registerMap(in map[string]dto.Decimal) (map[energy.Register]decimal.Decimal, error) {
	if in == nil {
		return nil, nil
	}
	valid := map[string]bool{}
	for _, reg := range energy.AllRegisters() {
		valid[string(reg)] = true
	}
	out := make(map[energy.Register]decimal.Decimal, len(in))
	for k, v := range in {
		if !valid[k] {
			return nil, kit.ErrInvalidParameters.WithParams(map[string]any{k: []string{"unknown_register"}})
		}
		out[energy.Register(k)] = v.Decimal
	}
	return out, nil
}

func (h *Handlers) resolveAnomaly(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AnomalyResolveRequest) (any, error) {
		after, err := registerMap(q.ResetAfter)
		if err != nil {
			return nil, err
		}
		overrides, err := registerMap(q.Overrides)
		if err != nil {
			return nil, err
		}
		a, err := h.Analysis.ResolveAnomaly(r.Context(), mw.ScopeFrom(r), q.ID, principal(r).User.ID, consumption.Resolution{
			Mode: consumption.ResolutionMode(q.Mode), ResetTS: q.ResetTs, ResetAfter: after, Overrides: overrides,
		})
		return anomalyDTO(a), err
	})
}

func (h *Handlers) reactiveStatus(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ReactiveStatusQuery) (any, error) {
		var month time.Time
		if q.Month != "" {
			m, err := time.ParseInLocation("2006-01", q.Month, dto.Istanbul)
			if err != nil {
				return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"month": []string{"invalid"}})
			}
			month = m
		}
		s, err := h.Analysis.ReactiveStatus(r.Context(), mw.ScopeFrom(r), month, q.BuildingID)
		if err != nil {
			return nil, err
		}
		out := dto.ReactiveStatus{Month: s.Month, Analyzers: make([]dto.ReactiveAnalyzer, len(s.Analyzers))}
		for i, a := range s.Analyzers {
			view := dto.ReactiveAnalyzer{AnalyzerID: a.AnalyzerID, BuildingID: a.BuildingID, HasData: a.HasData,
				InductiveRatio: decOrNil(a.InductiveRatio), CapacitiveRatio: decOrNil(a.CapacitiveRatio),
				InductiveLimit: decOrNil(a.InductiveLimit), CapacitiveLimit: decOrNil(a.CapacitiveLimit),
				InstalledPowerKw: decOrNil(a.InstalledPowerKw), PenaltyApplies: a.PenaltyApplies}
			if a.ExemptReason != "" {
				reason := a.ExemptReason
				view.ExemptReason = &reason
			}
			out.Analyzers[i] = view
		}
		extreme := func(e *analysis.ReactiveExtreme) *dto.ReactiveExtreme {
			if e == nil {
				return nil
			}
			return &dto.ReactiveExtreme{AnalyzerID: e.AnalyzerID, BuildingID: e.BuildingID, Ratio: dto.D(e.Ratio)}
		}
		out.HighestInductive, out.HighestCapacitive = extreme(s.HighestInductive), extreme(s.HighestCapacitive)
		return out, nil
	})
}

func profileKeys(names []string) ([]domainlp.Key, error) {
	valid := map[string]bool{}
	for _, k := range analysis.ProfileKeys() {
		valid[string(k)] = true
	}
	out := make([]domainlp.Key, 0, len(names))
	for _, n := range names {
		if !valid[n] {
			return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"profiles": []string{"invalid"}})
		}
		out = append(out, domainlp.Key(n))
	}
	return out, nil
}

func (h *Handlers) profiles(r *http.Request, q dto.LoadProfileQuery) (loadprofile.Result, error) {
	keys, err := profileKeys(q.Profiles)
	if err != nil {
		return loadprofile.Result{}, err
	}
	return h.Analysis.Profiles(r.Context(), mw.ScopeFrom(r), analysis.ProfileInput{AnalyzerID: q.AnalyzerID, From: q.From.Time, To: q.To.Time, Keys: keys})
}

func profileConfig(res loadprofile.Result) dto.LoadProfileConfig {
	days := make([]int, len(res.Config.WeekendDays))
	for i, d := range res.Config.WeekendDays {
		days[i] = int(d)
	}
	return dto.LoadProfileConfig{WeekendDays: days, WeekendSource: res.Config.WeekendSource, Vacations: res.Config.Vacations}
}

func (h *Handlers) loadProfiles(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.LoadProfileQuery) (any, error) {
		res, err := h.profiles(r, q)
		if err != nil {
			return nil, err
		}
		out := dto.LoadProfiles{Profiles: map[string][]*dto.Decimal{}, Days: map[string]int{}, Config: profileConfig(res)}
		for key, p := range res.Profiles {
			hours := make([]*dto.Decimal, 24)
			for i, v := range p.Hours {
				hours[i] = decOrNil(v)
			}
			out.Profiles[string(key)] = hours
			out.Days[string(key)] = p.Days
		}
		return out, nil
	})
}

func (h *Handlers) loadProfileStatistics(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.LoadProfileQuery) (any, error) {
		res, err := h.profiles(r, q)
		if err != nil {
			return nil, err
		}
		out := dto.LoadProfileStatistics{Statistics: map[string]dto.ProfileStatistics{}, Config: profileConfig(res)}
		for key, s := range res.Statistics {
			out.Statistics[string(key)] = dto.ProfileStatistics{Max: decOrNil(s.Max), Min: decOrNil(s.Min), HourOfMax: s.HourOfMax,
				Mean: decOrNil(s.Mean), StdDev: decOrNil(s.StdDev), Range: decOrNil(s.Range), LoadFactor: decOrNil(s.LoadFactor)}
		}
		return out, nil
	})
}

func (h *Handlers) loadProfileExport(w http.ResponseWriter, r *http.Request) {
	var q dto.LoadProfileQuery
	if err := kit.Bind(r, &q); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	res, err := h.profiles(r, q)
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	columns := []string{"hour"}
	var keys []domainlp.Key
	for _, k := range analysis.ProfileKeys() {
		if _, ok := res.Profiles[k]; ok {
			keys = append(keys, k)
			columns = append(columns, string(k))
		}
	}
	rows := make([][]string, 24)
	for hour := range 24 {
		row := []string{fmt.Sprintf("%02d:00", hour)}
		for _, k := range keys {
			v := res.Profiles[k].Hours[hour]
			if v == nil {
				row = append(row, "")
			} else {
				row = append(row, v.String())
			}
		}
		rows[hour] = row
	}
	body, err := table.XLSX("Yuk Profili", columns, rows)
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	writeFile(w, xlsxType, fmt.Sprintf("yuk-profili-%s-%s.xlsx", q.From.Format("20060102"), q.To.Format("20060102")), body)
}

func (h *Handlers) anomalyCheck(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AnomalyCheckRequest) (any, error) {
		if _, err := h.Analysis.Analyzer(r.Context(), mw.ScopeFrom(r), q.AnalyzerID); err != nil {
			return nil, err
		}
		return dto.AnomalyCheck{Available: false, Reason: "ml_service_unavailable"}, nil
	})
}

func (h *Handlers) energyBalance(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ConsumptionQuery) (any, error) {
		rows, _, err := h.Analysis.Rows(r.Context(), mw.ScopeFrom(r), seriesInput(q))
		if err != nil {
			return nil, err
		}
		balance := consumption.Balance(rows)
		out := dto.EnergyBalance{Items: make([]dto.BalanceRow, len(balance))}
		for i, b := range balance {
			out.Items[i] = dto.BalanceRow{PeriodStart: dto.T(b.Window.From), PeriodEnd: dto.T(b.Window.To), Consumption: decOrNil(b.Consumption),
				Generation: decOrNil(b.Generation), GridImport: decOrNil(b.GridImport), GridExport: decOrNil(b.GridExport)}
		}
		return out, nil
	})
}

func (h *Handlers) consumptionGrouped(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ConsumptionGroupedQuery) (any, error) {
		out, err := h.Analysis.Grouped(r.Context(), mw.ScopeFrom(r), analysis.GroupedInput{
			Subject: analysis.Subject{AnalyzerID: q.AnalyzerID, BuildingID: q.BuildingID},
			From:    q.From.Time, To: q.To.Time, By: grouping.By(q.GroupBy), ComparePrevious: q.Compare == "previous",
		})
		if err != nil {
			return nil, err
		}
		res := dto.ConsumptionGrouped{GroupBy: string(out.By), Current: groupedPeriodDTO(out.Current)}
		if out.Previous != nil {
			previous := groupedPeriodDTO(*out.Previous)
			res.Previous = &previous
		}
		return res, nil
	})
}

func groupedPeriodDTO(p analysis.GroupedPeriod) dto.GroupedPeriod {
	groups := make([]dto.GroupedBucket, 0, len(p.Buckets))
	for _, b := range p.Buckets {
		groups = append(groups, dto.GroupedBucket{
			Key: b.Key, Days: b.Days, ActiveImport: decOrNil(b.Active), ReactiveInductiveImport: decOrNil(b.Inductive),
			ReactiveCapacitiveImport: decOrNil(b.Capacitive), Partial: b.Partial,
		})
	}
	return dto.GroupedPeriod{
		From: dto.Date{Time: p.From}, To: dto.Date{Time: p.To}, Groups: groups,
		Statistics: dto.GroupedStatistics{
			Total: decOrNil(p.Statistics.Total), Average: decOrNil(p.Statistics.Average),
			Peak: extremeDTO(p.Statistics.Peak), Valley: extremeDTO(p.Statistics.Valley),
		},
	}
}

func extremeDTO(e *grouping.Extreme) *dto.GroupedExtreme {
	if e == nil {
		return nil
	}
	return &dto.GroupedExtreme{Key: e.Key, Value: dto.D(e.Value)}
}
