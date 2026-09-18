package v1

import (
	"net/http"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func alarmRoutes() []Route {
	all := auth.AllRoles
	write := auth.Roles(roleA, roleCA, roleBA)
	evaluate := auth.Roles(roleA, roleCA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/alarms", OperationID: "alarms.list", Tag: "alarms", Access: RoleGated, Roles: all,
			Summary: "Alarm rules with their analyzers and channels.", Request: dto.AlarmListRequest{},
			Response: dto.Page[dto.Alarm]{}, Status: http.StatusOK, Handler: (*Handlers).listAlarms},
		{Method: http.MethodPost, Pattern: "/alarms", OperationID: "alarms.create", Tag: "alarms", Access: RoleGated, Roles: write,
			Entity: "alarm", Summary: "Create a rule; the body is validated per alarm type.", Request: dto.AlarmFields{},
			Response: dto.Alarm{}, Status: http.StatusCreated, Handler: (*Handlers).createAlarm},
		{Method: http.MethodGet, Pattern: "/alarms/{id}", OperationID: "alarms.get", Tag: "alarms", Access: RoleGated, Roles: all,
			Summary: "One rule.", Request: dto.IDPath{}, Response: dto.Alarm{}, Status: http.StatusOK, Handler: (*Handlers).getAlarm},
		{Method: http.MethodPatch, Pattern: "/alarms/{id}", OperationID: "alarms.update", Tag: "alarms", Access: RoleGated, Roles: write,
			Entity: "alarm", Summary: "Replace a rule, including the enable toggle.", Request: dto.AlarmUpdateRequest{},
			Response: dto.Alarm{}, Status: http.StatusOK, Handler: (*Handlers).updateAlarm},
		{Method: http.MethodDelete, Pattern: "/alarms/{id}", OperationID: "alarms.delete", Tag: "alarms", Access: RoleGated, Roles: write,
			Entity: "alarm", Summary: "Soft delete a rule.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).deleteAlarm},
		{Method: http.MethodGet, Pattern: "/alarms/{id}/events", OperationID: "alarms.events.list", Tag: "alarms", Access: RoleGated,
			Roles: all, Summary: "Firing history.", Request: dto.AlarmEventsRequest{}, Response: dto.Page[dto.AlarmEvent]{},
			Status: http.StatusOK, Handler: (*Handlers).listAlarmEvents},
		// NoIdempotency (R223): a second deliberate dry run must be allowed to
		// see fresh data rather than replaying the first one's answer.
		{Method: http.MethodPost, Pattern: "/alarms/{id}/evaluate", OperationID: "alarms.evaluate", Tag: "alarms", Access: RoleGated,
			Roles: evaluate, Entity: "alarm", NoIdempotency: true,
			Summary: "Evaluate now; a dry run unless notify=true.", Request: dto.AlarmEvaluateRequest{},
			Response: dto.AlarmEvaluation{}, Status: http.StatusOK, Handler: (*Handlers).evaluateAlarm},
	}
}

func alarmSettingsDTO(a model.Alarm) dto.AlarmSettings {
	// Only the fields this type gives meaning to are rendered, so a stale
	// column from a rule whose type was changed cannot leak into the response.
	var out dto.AlarmSettings
	switch a.Type {
	case model.AlarmTypeReactiveLimit:
		out.InductiveRatioThreshold, out.InductivePeriodValue = dto.DP(a.InductiveRatioThreshold), a.InductivePeriodValue
		out.InductivePeriodUnit = periodUnitDTO(a.InductivePeriodUnit)
		out.CapacitiveRatioThreshold, out.CapacitivePeriodValue = dto.DP(a.CapacitiveRatioThreshold), a.CapacitivePeriodValue
		out.CapacitivePeriodUnit = periodUnitDTO(a.CapacitivePeriodUnit)
		out.ActiveConsumptionMax, out.ActiveConsumptionMaxPeriodValue = dto.DP(a.ActiveConsumptionMax), a.ActiveConsumptionMaxPeriodValue
		out.ActiveConsumptionMaxPeriodUnit = periodUnitDTO(a.ActiveConsumptionMaxPeriodUnit)
		out.ActiveConsumptionMin, out.ActiveConsumptionMinPeriodValue = dto.DP(a.ActiveConsumptionMin), a.ActiveConsumptionMinPeriodValue
		out.ActiveConsumptionMinPeriodUnit = periodUnitDTO(a.ActiveConsumptionMinPeriodUnit)
	case model.AlarmTypeDataCommunication:
		out.CommunicationThresholdHours = a.CommunicationThresholdHours
	case model.AlarmTypeCurrentVoltagePower:
		// R212: voltage is never rendered, so no client can round-trip one back.
		out.PowerMax, out.PowerMin = dto.DP(a.PowerMax), dto.DP(a.PowerMin)
	case model.AlarmTypeInvoiceIncrease:
		out.InvoiceThresholdPct = dto.DP(a.InvoiceThresholdPct)
	}
	return out
}

func periodUnitDTO(u *model.PeriodUnit) *string {
	if u == nil {
		return nil
	}
	s := string(*u)
	return &s
}

func periodUnitModel(s *string) *model.PeriodUnit {
	if s == nil {
		return nil
	}
	u := model.PeriodUnit(*s)
	return &u
}

func alarmDTO(r alarms.Rule) dto.Alarm {
	out := dto.Alarm{
		ID: r.Alarm.ID, Name: r.Alarm.Name, Type: string(r.Alarm.Type), IsEnabled: r.Alarm.IsEnabled,
		Analyzers:                  make([]dto.AlarmAnalyzer, 0, len(r.Analyzers)),
		Channels:                   make([]dto.AlarmChannel, 0, len(r.Channels)),
		NotificationFrequencyValue: r.Alarm.NotificationFrequencyValue,
		NotificationFrequencyUnit:  periodUnitDTO(r.Alarm.NotificationFrequencyUnit),
		Settings:                   alarmSettingsDTO(r.Alarm),
		CreatedAt:                  dto.T(r.Alarm.CreatedAt), UpdatedAt: dto.T(r.Alarm.UpdatedAt),
	}
	for _, a := range r.Analyzers {
		out.Analyzers = append(out.Analyzers, dto.AlarmAnalyzer{
			ID: a.ID, InstallationNumber: a.InstallationNumber, BuildingID: a.BuildingID})
	}
	for _, c := range r.Channels {
		out.Channels = append(out.Channels, dto.AlarmChannel{Channel: string(c.Channel), Target: c.Target})
	}
	return out
}

func alarmInput(f dto.AlarmFields) alarms.Input {
	dp := func(d *dto.Decimal) *decimal.Decimal {
		if d == nil {
			return nil
		}
		v := d.Decimal
		return &v
	}
	s := f.Settings
	in := alarms.Input{
		Name: f.Name, Type: model.AlarmType(f.Type), IsEnabled: f.IsEnabled, AnalyzerIDs: f.AnalyzerIDs,
		Alarm: model.Alarm{
			InductiveRatioThreshold: dp(s.InductiveRatioThreshold), InductivePeriodValue: s.InductivePeriodValue,
			InductivePeriodUnit:      periodUnitModel(s.InductivePeriodUnit),
			CapacitiveRatioThreshold: dp(s.CapacitiveRatioThreshold), CapacitivePeriodValue: s.CapacitivePeriodValue,
			CapacitivePeriodUnit: periodUnitModel(s.CapacitivePeriodUnit),
			ActiveConsumptionMax: dp(s.ActiveConsumptionMax), ActiveConsumptionMaxPeriodValue: s.ActiveConsumptionMaxPeriodValue,
			ActiveConsumptionMaxPeriodUnit: periodUnitModel(s.ActiveConsumptionMaxPeriodUnit),
			ActiveConsumptionMin:           dp(s.ActiveConsumptionMin), ActiveConsumptionMinPeriodValue: s.ActiveConsumptionMinPeriodValue,
			ActiveConsumptionMinPeriodUnit: periodUnitModel(s.ActiveConsumptionMinPeriodUnit),
			CommunicationThresholdHours:    s.CommunicationThresholdHours,
			PowerMax:                       dp(s.PowerMax), PowerMin: dp(s.PowerMin),
			// R212: carried through ONLY so alarm.Validate can refuse them by
			// name. settingsFor never copies them onto the stored row.
			VoltageMax: dp(s.VoltageMax), VoltageMin: dp(s.VoltageMin),
			InvoiceThresholdPct:        dp(s.InvoiceThresholdPct),
			NotificationFrequencyValue: f.NotificationFrequencyValue,
			NotificationFrequencyUnit:  periodUnitModel(f.NotificationFrequencyUnit),
		},
	}
	for _, c := range f.Channels {
		in.Channels = append(in.Channels, model.AlarmChannel{Channel: model.NotifyChannel(c.Channel), Target: c.Target})
	}
	return in
}

func (h *Handlers) listAlarms(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AlarmListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(q.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.AlarmFilter{IsEnabled: q.IsEnabled, AnalyzerID: q.AnalyzerID, Page: page}
		if q.Type != nil {
			f.Types = []model.AlarmType{model.AlarmType(*q.Type)}
		}
		list, err := h.Alarms.List(r.Context(), mw.ScopeFrom(r), f)
		if err != nil {
			return nil, err
		}
		items := make([]dto.Alarm, 0, len(list))
		for _, rule := range list {
			items = append(items, alarmDTO(rule))
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) getAlarm(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.IDPath) (any, error) {
		rule, err := h.Alarms.Get(r.Context(), mw.ScopeFrom(r), q.ID)
		if err != nil {
			return nil, err
		}
		return alarmDTO(rule), nil
	})
}

func (h *Handlers) createAlarm(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(f dto.AlarmFields) (any, error) {
		rule, err := h.Alarms.Create(r.Context(), mw.ScopeFrom(r), alarmInput(f))
		if err != nil {
			return nil, err
		}
		return alarmDTO(rule), nil
	})
}

func (h *Handlers) updateAlarm(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AlarmUpdateRequest) (any, error) {
		rule, err := h.Alarms.Update(r.Context(), mw.ScopeFrom(r), q.ID, alarmInput(q.AlarmFields))
		if err != nil {
			return nil, err
		}
		return alarmDTO(rule), nil
	})
}

func (h *Handlers) deleteAlarm(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(q dto.IDPath) (any, error) {
		return nil, h.Alarms.Delete(r.Context(), mw.ScopeFrom(r), q.ID)
	})
}

func (h *Handlers) listAlarmEvents(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AlarmEventsRequest) (any, error) {
		page, limit, err := kit.ResolvePage(q.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		f := store.AlarmEventFilter{Page: page}
		if q.From != nil && q.To != nil {
			f.Range = &store.TimeRange{From: *q.From, To: *q.To}
		}
		list, err := h.Alarms.Events(r.Context(), mw.ScopeFrom(r), q.ID, f)
		if err != nil {
			return nil, err
		}
		items := make([]dto.AlarmEvent, 0, len(list))
		for _, e := range list {
			items = append(items, dto.AlarmEvent{
				ID: e.ID, AnalyzerID: e.AnalyzerID, TriggeredAt: dto.T(e.TriggeredAt), Message: e.Message,
				Detail: e.Detail, NotifiedAt: dto.TP(e.NotifiedAt), NotificationError: e.NotificationError})
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) evaluateAlarm(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.AlarmEvaluateRequest) (any, error) {
		ev, err := h.Alarms.DryRun(r.Context(), mw.ScopeFrom(r), q.ID, q.Notify)
		if err != nil {
			return nil, err
		}
		out := dto.AlarmEvaluation{
			EvaluatedAt: dto.T(ev.EvaluatedAt), DryRun: ev.DryRun, NotificationsSent: ev.NotificationsSent,
			Analyzers: make([]dto.AlarmEvaluationAnalyzer, 0, len(ev.Analyzers)),
		}
		for _, a := range ev.Analyzers {
			row := dto.AlarmEvaluationAnalyzer{
				AnalyzerID: a.AnalyzerID, InstallationNumber: a.InstallationNumber, Fired: a.Verdict.Fired(),
				Breaches:  make([]dto.AlarmEvaluationBreach, 0, len(a.Verdict.Breaches)),
				NoVerdict: a.Verdict.NoVerdict,
			}
			if row.NoVerdict == nil {
				row.NoVerdict = []string{}
			}
			for i, b := range a.Verdict.Breaches {
				message := ""
				if i < len(a.Lines) {
					message = a.Lines[i]
				}
				row.Breaches = append(row.Breaches, dto.AlarmEvaluationBreach{
					Field: b.Field, Measured: dto.D(b.Measured), Threshold: dto.D(b.Threshold), Message: message})
			}
			out.Analyzers = append(out.Analyzers, row)
		}
		return out, nil
	})
}
