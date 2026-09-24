package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	weathersvc "github.com/MErenTalan/ekokod-rewrite/internal/service/weather"
)

// renewableRoutes is 05 §9's scope routes (R291–R293).
func renewableRoutes() []Route {
	all := auth.AllRoles
	return []Route{
		{Method: http.MethodGet, Pattern: "/weather", OperationID: "weather.get", Tag: "renewable", Access: RoleGated, Roles: all,
			Summary: "Current conditions and seven days for a plant's or building's coordinates; never a default city.",
			Request: dto.WeatherRequest{}, Response: dto.Weather{}, Status: http.StatusOK, Handler: (*Handlers).weather},
	}
}

func weatherDTO(v weathersvc.View) dto.Weather {
	out := dto.Weather{Available: v.Available, Reason: v.Reason, Days: make([]dto.WeatherDay, len(v.Days))}
	for i, d := range v.Days {
		out.Days[i] = dto.WeatherDay{Date: dto.Date{Time: d.Date}, MinC: dto.DP(d.MinC), MaxC: dto.DP(d.MaxC), WeatherCode: d.WeatherCode,
			PrecipitationPct: dto.DP(d.PrecipitationPct), UVIndexMax: dto.DP(d.UVIndexMax), ShortwaveMJ: dto.DP(d.ShortwaveMJ), Potential: d.Potential}
	}
	if v.Current != nil {
		c := &dto.WeatherCurrent{TemperatureC: dto.DP(v.Current.TemperatureC), HumidityPct: dto.DP(v.Current.HumidityPct),
			WindKmh: dto.DP(v.Current.WindKmh), PressureHpa: dto.DP(v.Current.PressureHpa), VisibilityKm: dto.DP(v.Current.VisibilityKm),
			WeatherCode: v.Current.WeatherCode}
		// §7.8 lists UV and precipitation chance with the current conditions: today's forecast carries them.
		if len(v.Days) > 0 {
			c.UVIndex, c.PrecipitationPct = dto.DP(v.Days[0].UVIndexMax), dto.DP(v.Days[0].PrecipitationPct)
		}
		out.Current = c
		out.PotentialBasis = "shortwave_radiation_sum_mj_m2"
	}
	return out
}

func (h *Handlers) weather(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.WeatherRequest) (any, error) {
		v, err := h.Weather.For(r.Context(), mw.ScopeFrom(r), req.PlantID, req.BuildingID)
		if err != nil {
			return nil, err
		}
		return weatherDTO(v), nil
	})
}
