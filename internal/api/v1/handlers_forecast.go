package v1

import (
	"errors"
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	forecastsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/forecast"
)

// forecastRoutes is 05 §16 (F13b R370–R379): reads for every scope, runs for A CA BA.
func forecastRoutes() []Route {
	writers := auth.Roles(roleA, roleCA, roleBA)
	post := func(pattern, id, summary string, req any, h func(*Handlers, http.ResponseWriter, *http.Request), idempotent bool) Route {
		return Route{Method: http.MethodPost, Pattern: pattern, OperationID: id, Tag: "forecast", Access: RoleGated, Roles: writers,
			Entity: "analyzer", NoIdempotency: !idempotent, Summary: summary, Request: req, Response: dto.Forecast{}, Status: http.StatusOK, Handler: h}
	}
	return []Route{
		{Method: http.MethodGet, Pattern: "/forecast", OperationID: "forecast.read", Tag: "forecast", Access: RoleGated, Roles: auth.AllRoles,
			Summary: "The latest stored forecast covering the window, with its gaps; never calls the ML service.",
			Request: dto.ForecastQuery{}, Response: dto.Forecast{}, Status: http.StatusOK, Handler: (*Handlers).forecastRead},
		post("/forecast/run", "forecast.run", "Forecast an analyzer now and store the run.", dto.ForecastRunRequest{}, (*Handlers).forecastRun, true),
		post("/forecast/weekly", "forecast.weekly", "An hourly forecast for one week (not stored).", dto.ForecastWeeklyRequest{}, (*Handlers).forecastWeekly, false),
		post("/forecast/monthly", "forecast.monthly", "A daily forecast for one month (not stored).", dto.ForecastMonthlyRequest{}, (*Handlers).forecastMonthly, false),
		{Method: http.MethodPost, Pattern: "/anomaly/check", OperationID: "anomaly.check", Tag: "forecast", Access: RoleGated,
			Roles: writers, Entity: "analyzer", NoIdempotency: true,
			Summary: "Check one hour's consumption against the model; available=false when the ML service is down.",
			Request: dto.AnomalyCheckRequest{}, Response: dto.AnomalyCheck{}, Status: http.StatusOK, Handler: (*Handlers).anomalyCheck},
	}
}

func forecastOut(res forecastsvc.Result) dto.Forecast {
	out := dto.Forecast{Status: res.Status, GeneratedAt: res.GeneratedAt, FallbackFrom: res.FallbackFrom,
		UsedCovariates: append([]string{}, res.UsedCovariates...), Points: []dto.ForecastPoint{}, Gaps: []dto.ForecastGap{}}
	if res.ModelID != "" {
		out.ModelID, out.ModelVersion = &res.ModelID, &res.ModelVersion
	}
	for _, p := range res.Points {
		out.Points = append(out.Points, dto.ForecastPoint{Ts: p.Ts, Median: dto.D(p.Median), P10: decPtr(p.P10), P90: decPtr(p.P90)})
	}
	for _, g := range res.Gaps {
		out.Gaps = append(out.Gaps, dto.ForecastGap{Start: g.GapStart, End: g.GapEnd, MissingHours: int(g.MissingHours)})
	}
	return out
}

func decPtr(d *decimal.Decimal) *dto.Decimal {
	if d == nil {
		return nil
	}
	v := dto.D(*d)
	return &v
}

func (h *Handlers) forecastRead(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ForecastQuery) (any, error) {
		res, err := h.Forecast.Read(r.Context(), mw.ScopeFrom(r), q.AnalyzerID, *q.From, *q.To)
		return forecastOut(res), err
	})
}

func (h *Handlers) forecastRun(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ForecastRunRequest) (any, error) {
		res, err := h.Forecast.Run(r.Context(), mw.ScopeFrom(r), q.AnalyzerID, q.HorizonHours)
		return forecastOut(res), err
	})
}

func (h *Handlers) forecastWeekly(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ForecastWeeklyRequest) (any, error) {
		res, err := h.Forecast.Weekly(r.Context(), mw.ScopeFrom(r), q.AnalyzerID, q.WeekStart.Time)
		return forecastOut(res), err
	})
}

func (h *Handlers) forecastMonthly(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.ForecastMonthlyRequest) (any, error) {
		res, err := h.Forecast.Monthly(r.Context(), mw.ScopeFrom(r), q.AnalyzerID, q.Month)
		return forecastOut(res), err
	})
}

func (h *Handlers) anomalyCheck(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AnomalyCheckRequest) (any, error) {
		var actual *decimal.Decimal
		if q.Actual != nil {
			actual = &q.Actual.Decimal
		}
		res, err := h.Forecast.Anomaly(r.Context(), mw.ScopeFrom(r), q.AnalyzerID, q.Ts, actual)
		if errors.Is(err, forecastsvc.ErrUnavailable) {
			return dto.AnomalyCheck{Available: false, Reason: "ml_service_unavailable"}, nil
		}
		if err != nil {
			return nil, err
		}
		out := dto.AnomalyCheck{Available: true, IsAnomaly: &res.IsAnomaly, Score: res.Score, Actual: decPtr(&res.Actual),
			Method: res.Method, ModelID: res.ModelID, ModelVersion: res.ModelVersion}
		for dst, src := range map[**dto.Decimal]*float64{&out.Expected: res.Expected, &out.Lower: res.Lower, &out.Upper: res.Upper} {
			if src != nil {
				v := dto.D(decimal.NewFromFloat(*src).Round(4))
				*dst = &v
			}
		}
		return out, nil
	})
}
