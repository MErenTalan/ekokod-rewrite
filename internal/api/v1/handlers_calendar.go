package v1

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/calendar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func calendarRoutes() []Route {
	all := auth.AllRoles
	aca := auth.Roles(roleA, roleCA)
	a := auth.Roles(roleA)
	return []Route{
		{Method: http.MethodGet, Pattern: "/calendar/events", OperationID: "calendar.events.list", Tag: "calendar", Access: RoleGated, Roles: all,
			Summary: "Events overlapping a date range.", Request: dto.CalendarEventsRequest{}, Response: dto.CalendarEvents{}, Status: http.StatusOK,
			Handler: (*Handlers).listEvents},
		{Method: http.MethodPost, Pattern: "/calendar/events", OperationID: "calendar.events.create", Tag: "calendar", Access: RoleGated, Roles: aca,
			Entity: "calendar_event", Summary: "Create an event.", Request: dto.CalendarEventFields{}, Response: dto.CalendarEvent{},
			Status: http.StatusCreated, Handler: (*Handlers).createEvent},
		{Method: http.MethodPatch, Pattern: "/calendar/events/{id}", OperationID: "calendar.events.update", Tag: "calendar", Access: RoleGated,
			Roles: aca, Entity: "calendar_event", Summary: "Replace an event.", Request: dto.CalendarEventUpdateRequest{},
			Response: dto.CalendarEvent{}, Status: http.StatusOK, Handler: (*Handlers).updateEvent},
		{Method: http.MethodDelete, Pattern: "/calendar/events/{id}", OperationID: "calendar.events.delete", Tag: "calendar", Access: RoleGated,
			Roles: aca, Entity: "calendar_event", Summary: "Delete an event.", Request: dto.IDPath{}, Status: http.StatusNoContent,
			Handler: (*Handlers).deleteEvent},
		{Method: http.MethodGet, Pattern: "/calendar/vacations", OperationID: "calendar.vacations.get", Tag: "calendar", Access: RoleGated, Roles: all,
			Summary: "Weekend days and vacation periods.", Response: dto.Vacations{}, Status: http.StatusOK, Handler: (*Handlers).getVacations},
		{Method: http.MethodPut, Pattern: "/calendar/vacations", OperationID: "calendar.vacations.put", Tag: "calendar", Access: RoleGated, Roles: aca,
			Entity: "company_vacations", Summary: "Replace weekend days and vacation periods.", Request: dto.VacationsPutRequest{},
			Response: dto.Vacations{}, Status: http.StatusOK, Handler: (*Handlers).putVacations},

		{Method: http.MethodGet, Pattern: "/integration-definitions", OperationID: "integration_definitions.list", Tag: "integrations",
			Access: RoleGated, Roles: a, Summary: "The provider catalogue.", Response: dto.IntegrationDefinitions{}, Status: http.StatusOK,
			Handler: (*Handlers).listDefinitions},
		{Method: http.MethodPost, Pattern: "/integration-definitions", OperationID: "integration_definitions.create", Tag: "integrations",
			Access: RoleGated, Roles: a, Entity: "integration_definition", PlatformAudit: true, Summary: "Add a provider/subtype.",
			Request: dto.IntegrationDefinitionCreateRequest{}, Response: dto.IntegrationDefinition{}, Status: http.StatusCreated,
			Handler: (*Handlers).createDefinition},
		{Method: http.MethodPatch, Pattern: "/integration-definitions/{id}", OperationID: "integration_definitions.update", Tag: "integrations",
			Access: RoleGated, Roles: a, Entity: "integration_definition", PlatformAudit: true, Summary: "Update a definition.",
			Request: dto.IntegrationDefinitionUpdateRequest{}, Response: dto.IntegrationDefinition{}, Status: http.StatusOK,
			Handler: (*Handlers).updateDefinition},
		{Method: http.MethodDelete, Pattern: "/integration-definitions/{id}", OperationID: "integration_definitions.delete", Tag: "integrations",
			Access: RoleGated, Roles: a, Entity: "integration_definition", PlatformAudit: true, Summary: "Delete an unused definition.",
			Request: dto.IDPath{}, Status: http.StatusNoContent, Handler: (*Handlers).deleteDefinition},
		{Method: http.MethodGet, Pattern: "/integration-credentials", OperationID: "integration_credentials.list", Tag: "integrations",
			Access: RoleGated, Roles: aca, Summary: "Configured integrations; never secrets.", Response: dto.IntegrationCredentials{},
			Status: http.StatusOK, Handler: (*Handlers).listCredentials},
		{Method: http.MethodPost, Pattern: "/integration-credentials", OperationID: "integration_credentials.create", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", Summary: "Configure an integration; secrets are write-only.",
			Request: dto.IntegrationCredentialCreateRequest{}, Response: dto.IntegrationCredential{}, Status: http.StatusCreated,
			Handler: (*Handlers).createCredential},
		{Method: http.MethodPatch, Pattern: "/integration-credentials/{id}", OperationID: "integration_credentials.update", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", Summary: "Update; omitted secrets are kept.",
			Request: dto.IntegrationCredentialUpdateRequest{}, Response: dto.IntegrationCredential{}, Status: http.StatusOK,
			Handler: (*Handlers).updateCredential},
		{Method: http.MethodDelete, Pattern: "/integration-credentials/{id}", OperationID: "integration_credentials.delete", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", Summary: "Remove an integration.", Request: dto.IDPath{},
			Status: http.StatusNoContent, Handler: (*Handlers).deleteCredential},
		{Method: http.MethodPost, Pattern: "/integration-credentials/{id}/verify", OperationID: "integration_credentials.verify", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", NoIdempotency: true, Summary: "Test authentication against the provider.",
			Request: dto.IDPath{}, Response: dto.IntegrationCredential{}, Status: http.StatusOK, Handler: (*Handlers).verifyCredential},
		{Method: http.MethodPost, Pattern: "/integration-credentials/{id}/discover", OperationID: "integration_credentials.discover", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", Summary: "Discover metering points.", Request: dto.IDPath{},
			Response: dto.JobAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).discoverCredential},
		{Method: http.MethodPost, Pattern: "/integration-credentials/{id}/backfill", OperationID: "integration_credentials.backfill", Tag: "integrations",
			Access: RoleGated, Roles: aca, Entity: "integration_credential", Summary: "Enqueue a historical pull.", Request: dto.BackfillRequest{},
			Response: dto.JobAccepted{}, Status: http.StatusAccepted, Handler: (*Handlers).backfillCredential},
		{Method: http.MethodGet, Pattern: "/integrations/isolar/authorize-url", OperationID: "isolar.authorize_url", Tag: "integrations",
			Access: RoleGated, Roles: aca, Summary: "The iSolarCloud authorisation URL; binds the flow to this browser (R187).",
			Request: dto.AuthorizeURLRequest{}, Response: dto.AuthorizeURL{}, Status: http.StatusOK, Handler: (*Handlers).isolarAuthorizeURL},
		{Method: http.MethodGet, Pattern: "/integrations/isolar/callback", OperationID: "isolar.callback", Tag: "integrations", Access: Public,
			Summary: "OAuth callback; redirects to Settings.", Status: http.StatusFound, Handler: (*Handlers).isolarCallback},
	}
}

func eventDTO(e model.CalendarEvent) dto.CalendarEvent {
	return dto.CalendarEvent{ID: e.ID, Title: e.Title, StartsAt: dto.T(e.StartsAt), EndsAt: dto.T(e.EndsAt), AllDay: e.AllDay, Colour: e.Colour}
}

func eventInput(f dto.CalendarEventFields) calendar.EventInput {
	return calendar.EventInput{Title: f.Title, StartsAt: f.StartsAt, EndsAt: f.EndsAt, AllDay: f.AllDay, Colour: f.Colour}
}

func (h *Handlers) listEvents(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.CalendarEventsRequest) (any, error) {
		list, err := h.Calendar.ListEvents(r.Context(), mw.ScopeFrom(r), q.From.Time, q.To.AddDate(0, 0, 1))
		if err != nil {
			return nil, err
		}
		out := dto.CalendarEvents{Items: make([]dto.CalendarEvent, len(list))}
		for i, e := range list {
			out.Items[i] = eventDTO(e)
		}
		return out, nil
	})
}

func (h *Handlers) createEvent(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(f dto.CalendarEventFields) (any, error) {
		e, err := h.Calendar.CreateEvent(r.Context(), mw.ScopeFrom(r), principal(r).User.ID, eventInput(f))
		return eventDTO(e), err
	})
}

func (h *Handlers) updateEvent(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(f dto.CalendarEventUpdateRequest) (any, error) {
		e, err := h.Calendar.UpdateEvent(r.Context(), mw.ScopeFrom(r), f.ID, eventInput(f.CalendarEventFields))
		return eventDTO(e), err
	})
}

func (h *Handlers) deleteEvent(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(q dto.IDPath) (any, error) {
		return nil, h.Calendar.DeleteEvent(r.Context(), mw.ScopeFrom(r), q.ID)
	})
}

func vacationsDTO(v calendar.Vacations) dto.Vacations {
	out := dto.Vacations{WeekendDays: v.WeekendDays, WeekendSource: v.WeekendSource, Periods: make([]dto.VacationPeriod, len(v.Periods))}
	for i, p := range v.Periods {
		id := p.ID
		out.Periods[i] = dto.VacationPeriod{ID: &id, StartDate: dto.Date{Time: p.Start}, EndDate: dto.Date{Time: p.End}, Description: p.Description}
	}
	return out
}

func (h *Handlers) getVacations(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(struct{}) (any, error) {
		v, err := h.Calendar.GetVacations(r.Context(), mw.ScopeFrom(r))
		return vacationsDTO(v), err
	})
}

func (h *Handlers) putVacations(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.VacationsPutRequest) (any, error) {
		periods := make([]calendar.Period, len(req.Periods))
		for i, p := range req.Periods {
			periods[i] = calendar.Period{Start: p.StartDate.Time, End: p.EndDate.Time, Description: p.Description}
		}
		v, err := h.Calendar.PutVacations(r.Context(), mw.ScopeFrom(r), req.WeekendDays, periods)
		return vacationsDTO(v), err
	})
}

func definitionDTO(d model.IntegrationDefinition) dto.IntegrationDefinition {
	return dto.IntegrationDefinition{ID: d.ID, Provider: string(d.Provider), Subtype: d.Subtype, Endpoints: d.Endpoints, UpdatedAt: dto.T(d.UpdatedAt)}
}

func (h *Handlers) listDefinitions(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(struct{}) (any, error) {
		list, err := h.Definitions.List(r.Context(), mw.ScopeFrom(r))
		if err != nil {
			return nil, err
		}
		out := dto.IntegrationDefinitions{Items: make([]dto.IntegrationDefinition, len(list))}
		for i, d := range list {
			out.Items[i] = definitionDTO(d)
		}
		return out, nil
	})
}

func (h *Handlers) createDefinition(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.IntegrationDefinitionCreateRequest) (any, error) {
		d, err := h.Definitions.Create(r.Context(), model.IntegrationDefinition{
			Provider: model.IntegrationProvider(req.Provider), Subtype: req.Subtype, Endpoints: req.Endpoints,
		})
		return definitionDTO(d), err
	})
}

func (h *Handlers) updateDefinition(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IntegrationDefinitionUpdateRequest) (any, error) {
		d, err := h.Definitions.Update(r.Context(), model.IntegrationDefinition{ID: req.ID, Subtype: req.Subtype, Endpoints: req.Endpoints})
		return definitionDTO(d), err
	})
}

func (h *Handlers) deleteDefinition(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Definitions.Delete(r.Context(), req.ID)
	})
}

// credErr maps credential failures (R188): configuration and authentication
// problems stay distinguishable; an existing definition is a conflict.
func credErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, integration.ErrConfig):
		return perr.New("integration_config_invalid", 422, "errors.integrations.configInvalid")
	case errors.Is(err, integration.ErrAuth):
		return perr.New("integration_auth_failed", 422, "errors.integrations.authFailed")
	case errors.Is(err, integration.ErrRateLimited), errors.Is(err, integration.ErrUpstreamUnavailable):
		return perr.New("integration_unavailable", 503, "errors.integrations.unavailable")
	case errors.Is(err, credentials.ErrInvalidSettings), errors.Is(err, credentials.ErrInvalidExtraKey):
		return perr.Validation.WithParams(map[string]any{"settings": []string{"invalid"}})
	case errors.Is(err, credentials.ErrNotIsolar):
		return perr.Validation.WithParams(map[string]any{"credential_id": []string{"not_isolar"}})
	case errors.Is(err, store.ErrConflict):
		return perr.New("integration_already_configured", 409, "errors.integrations.alreadyConfigured")
	}
	return err
}

func credentialDTO(v credentials.View) dto.IntegrationCredential {
	keys := v.ExtraKeys
	if keys == nil {
		keys = []string{}
	}
	return dto.IntegrationCredential{ID: v.ID, DefinitionID: v.DefinitionID, Provider: string(v.Provider), Subtype: v.Subtype,
		Username: v.Username, HasSecret: v.HasSecret, ExtraKeys: keys, Settings: v.Settings, PM5340URL: v.PM5340URL,
		IsolarRegion: v.IsolarRegion, IsActive: v.IsActive, TokenExpiresAt: dto.TP(v.TokenExpiresAt), LastVerifiedAt: dto.TP(v.LastVerifiedAt),
		UpdatedAt: dto.T(v.UpdatedAt)}
}

func credentialInput(provider, subtype string, f dto.IntegrationCredentialFields) credentials.Input {
	in := credentials.Input{Provider: model.IntegrationProvider(provider), Subtype: subtype, Username: f.Username,
		Settings: f.Settings, PM5340URL: f.PM5340URL, InstallationNumber: f.InstallationNumber, IsolarRegion: f.IsolarRegion, IsActive: f.IsActive}
	if f.Secret != nil {
		secret := integration.NewSecret([]byte(*f.Secret))
		in.Secret = &secret
	}
	if f.Extra != nil {
		in.Extra = make(map[string]integration.Secret, len(f.Extra))
		for k, v := range f.Extra {
			in.Extra[k] = integration.NewSecret([]byte(v))
		}
	}
	return in
}

func (h *Handlers) listCredentials(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(struct{}) (any, error) {
		list, err := h.Credentials.List(r.Context(), mw.ScopeFrom(r))
		if err != nil {
			return nil, credErr(err)
		}
		out := dto.IntegrationCredentials{Items: make([]dto.IntegrationCredential, len(list))}
		for i, v := range list {
			out.Items[i] = credentialDTO(v)
		}
		return out, nil
	})
}

func (h *Handlers) createCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.IntegrationCredentialCreateRequest) (any, error) {
		v, err := h.Credentials.Configure(r.Context(), mw.ScopeFrom(r), credentialInput(req.Provider, req.Subtype, req.IntegrationCredentialFields))
		if err != nil {
			return nil, credErr(err)
		}
		return credentialDTO(v), nil
	})
}

func (h *Handlers) updateCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IntegrationCredentialUpdateRequest) (any, error) {
		v, err := h.Credentials.Update(r.Context(), mw.ScopeFrom(r), req.ID, credentialInput("", "", req.IntegrationCredentialFields))
		if err != nil {
			return nil, credErr(err)
		}
		return credentialDTO(v), nil
	})
}

func (h *Handlers) deleteCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, credErr(h.Credentials.Delete(r.Context(), mw.ScopeFrom(r), req.ID))
	})
}

func (h *Handlers) verifyCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		v, err := h.Credentials.Verify(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, credErr(err)
		}
		return credentialDTO(v), nil
	})
}

func (h *Handlers) discoverCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.IDPath) (any, error) {
		id, err := h.Credentials.Discover(r.Context(), mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, credErr(err)
		}
		return dto.JobAccepted{JobID: id}, nil
	})
}

func (h *Handlers) backfillCredential(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusAccepted, func(req dto.BackfillRequest) (any, error) {
		kinds := make([]model.ReadingKind, len(req.Kinds))
		for i, k := range req.Kinds {
			kinds[i] = model.ReadingKind(k)
		}
		id, err := h.Credentials.Backfill(r.Context(), mw.ScopeFrom(r), req.ID, credentials.BackfillInput{
			AnalyzerIDs: req.AnalyzerIDs, Kinds: kinds, From: req.From, To: req.To, Force: req.Force,
		})
		if err != nil {
			return nil, credErr(err)
		}
		return dto.JobAccepted{JobID: id}, nil
	})
}

// OAuthCookie binds an iSolar authorisation to the browser that started it (R187).
const OAuthCookie = "ekokod_oauth"

const callbackPath = "/api/v1/integrations/isolar/callback"

func stateHash(state string) string {
	sum := sha256.Sum256([]byte(state))
	return hex.EncodeToString(sum[:])
}

func (h *Handlers) isolarAuthorizeURL(w http.ResponseWriter, r *http.Request) {
	var q dto.AuthorizeURLRequest
	if err := kit.Bind(r, &q); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	u, state, err := h.Credentials.ISolarAuthorize(r.Context(), mw.ScopeFrom(r), q.CredentialID)
	if err != nil {
		kit.WriteError(w, r, credErr(err))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: OAuthCookie, Value: stateHash(state), Path: callbackPath, HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode, MaxAge: int((10 * time.Minute).Seconds())})
	kit.WriteJSON(w, http.StatusOK, dto.AuthorizeURL{URL: u})
}

func (h *Handlers) isolarCallback(w http.ResponseWriter, r *http.Request) {
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	outcome := "error"
	if c, err := r.Cookie(OAuthCookie); err == nil && state != "" && code != "" &&
		subtle.ConstantTimeCompare([]byte(c.Value), []byte(stateHash(state))) == 1 {
		if err := h.Credentials.ISolarCallback(r.Context(), code, state); err == nil {
			outcome = "ok"
		} else {
			h.Log.WarnContext(r.Context(), "api: isolar callback failed", "error", err.Error())
		}
	}
	http.SetCookie(w, &http.Cookie{Name: OAuthCookie, Value: "", Path: callbackPath, HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.Redirect(w, r, "/ekorm/settings?tab=company&isolar="+outcome, http.StatusFound)
}
