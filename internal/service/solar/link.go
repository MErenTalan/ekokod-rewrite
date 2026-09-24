package solar

import (
	"context"
	"errors"
	"net/mail"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Link-flow errors (R281, R288).
var (
	ErrAlreadyLinked = perr.New("isolar_plant_already_linked", 409, "errors.solar.alreadyLinked")
	ErrNotLinked     = perr.New("isolar_not_linked", 409, "errors.solar.notLinked")
)

// linkBackfillDays is R281's backfill on link: 13 daily-series calls.
const linkBackfillDays = 400

// maxRecipients bounds a plant's alarm recipients, as F6's PATCH does (R298).
const maxRecipients = 50

// AccountPlant is one plant on the connected iSolar account.
type AccountPlant struct {
	PSID          string
	Name          string
	InstalledKw   *decimal.Decimal
	LinkedPlantID *uuid.UUID
}

// openISolar opens an active company iSolar credential; anything else is
// not_isolar (R280), including another company's id, so the answer never
// confirms it exists.
func (s *Service) openISolar(ctx context.Context, sc store.Scope, credentialID uuid.UUID) (integration.Credentials, error) {
	creds, err := s.openActive(ctx, sc, credentialID)
	var se *syncError
	if errors.Is(err, store.ErrNotFound) || errors.As(err, &se) || (err == nil && creds.Provider != integration.ProviderISolar) {
		return integration.Credentials{}, validation("credential_id", "not_isolar")
	}
	return creds, err
}

// openActive opens a credential and refuses one an operator deactivated,
// before any network call and without retry (X-M3's ingest gate).
func (s *Service) openActive(ctx context.Context, sc store.Scope, credentialID uuid.UUID) (integration.Credentials, error) {
	creds, err := s.d.Creds.Open(ctx, sc, credentialID)
	if err == nil && !creds.IsActive {
		return integration.Credentials{}, &syncError{code: CodeCredentialInactive}
	}
	return creds, err
}

// AccountPlants lists the account's plants for the link modal.
func (s *Service) AccountPlants(ctx context.Context, sc store.Scope, credentialID uuid.UUID) ([]AccountPlant, error) {
	if !sc.AllBuildings {
		return nil, store.ErrNotFound
	}
	creds, err := s.openISolar(ctx, sc, credentialID)
	if err != nil {
		return nil, err
	}
	remote, err := s.d.ISolar.Plants(ctx, creds)
	if err != nil {
		return nil, err
	}
	ours, err := s.d.Plants.ListLinked(ctx, sc)
	if err != nil {
		return nil, err
	}
	linked := map[string]uuid.UUID{}
	for _, p := range ours {
		linked[*p.IsolarPsID] = p.ID
	}
	out := make([]AccountPlant, 0, len(remote))
	for _, r := range remote {
		ap := AccountPlant{PSID: r.PSID, Name: r.Name, InstalledKw: r.InstalledKw}
		if id, ok := linked[r.PSID]; ok {
			ap.LinkedPlantID = &id
		}
		out = append(out, ap)
	}
	return out, nil
}

// Link binds a plant to an account plant, imports it and enqueues a backfill (R280, R281).
func (s *Service) Link(ctx context.Context, sc store.Scope, plantID, credentialID uuid.UUID, psID string) (model.PowerPlant, string, error) {
	psID = strings.TrimSpace(psID)
	if psID == "" || len(psID) > 64 {
		return model.PowerPlant{}, "", validation("ps_id", "invalid")
	}
	plant, err := s.plantFor(ctx, sc, plantID)
	if err != nil {
		return model.PowerPlant{}, "", err
	}
	creds, err := s.openISolar(ctx, sc, credentialID)
	if err != nil {
		return model.PowerPlant{}, "", err
	}
	remote, err := s.d.ISolar.Plants(ctx, creds)
	if err != nil {
		return model.PowerPlant{}, "", err
	}
	var found *AccountPlant
	for _, r := range remote {
		if r.PSID == psID {
			found = &AccountPlant{PSID: r.PSID, Name: r.Name, InstalledKw: r.InstalledKw}
		}
	}
	if found == nil {
		return model.PowerPlant{}, "", validation("ps_id", "not_found")
	}
	now := s.d.Clock.Now()
	name := found.Name
	plant.IsolarPsID, plant.IsolarPsName, plant.IsolarInstalledKw = &found.PSID, &name, found.InstalledKw
	plant.IsolarLinkedAt, plant.IsolarCredentialID = &now, &credentialID
	if plant.TotalCapacityKw == nil {
		plant.TotalCapacityKw = found.InstalledKw
	}
	linked, err := s.d.Plants.SetIsolarLink(ctx, sc, plant)
	if errors.Is(err, store.ErrConflict) {
		return model.PowerPlant{}, "", ErrAlreadyLinked
	}
	if err != nil {
		return model.PowerPlant{}, "", err
	}
	devices, err := s.d.ISolar.Devices(ctx, creds, psID)
	if err != nil {
		return model.PowerPlant{}, "", err
	}
	for _, d := range devices {
		key := d.PSKey
		if _, err := s.d.Plants.UpsertDevice(ctx, sc, model.PlantDevice{PlantID: plantID, DeviceSN: d.DeviceSN, DeviceName: d.DeviceName,
			DeviceType: d.DeviceType, ProviderKey: &key}); err != nil {
			return model.PowerPlant{}, "", err
		}
	}
	jobID, err := s.enqueueSync(ctx, sc, plantID, linkBackfillDays)
	return linked, jobID, err
}

// Unlink clears the link and keeps stored production (R280).
func (s *Service) Unlink(ctx context.Context, sc store.Scope, plantID uuid.UUID) error {
	plant, err := s.plantFor(ctx, sc, plantID)
	if err != nil {
		return err
	}
	plant.IsolarPsID, plant.IsolarPsKey, plant.IsolarPsName, plant.IsolarInstalledKw = nil, nil, nil, nil
	plant.IsolarLinkedAt, plant.IsolarCredentialID = nil, nil
	_, err = s.d.Plants.SetIsolarLink(ctx, sc, plant)
	return err
}

// EnqueueSync is the screen's "update data" (R288).
func (s *Service) EnqueueSync(ctx context.Context, sc store.Scope, plantID uuid.UUID) (string, error) {
	plant, err := s.plantFor(ctx, sc, plantID)
	if err != nil {
		return "", err
	}
	if plant.IsolarPsID == nil {
		return "", ErrNotLinked
	}
	return s.enqueueSync(ctx, sc, plantID, 0)
}

func (s *Service) enqueueSync(ctx context.Context, sc store.Scope, plantID uuid.UUID, backfill int) (string, error) {
	p := job.SolarSyncPayload{CompanyID: sc.CompanyID, PlantID: plantID, BackfillDays: backfill}
	task, err := job.NewSolarSyncTask(p, job.TaskOptions{MaxRetry: s.d.MaxRetry})
	if err != nil {
		return "", err
	}
	id := job.SolarSyncTaskID(p)
	return id, job.EnqueueReplacingFinished(ctx, s.d.Enqueuer, s.d.Inspector, task, id)
}

// SetRecipients replaces the plant's alarm recipients (R298).
func (s *Service) SetRecipients(ctx context.Context, sc store.Scope, plantID uuid.UUID, emails []string) ([]string, error) {
	if _, err := s.plantFor(ctx, sc, plantID); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, e := range emails {
		addr := strings.ToLower(strings.TrimSpace(e))
		parsed, err := mail.ParseAddress(addr)
		if err != nil || parsed.Address != addr || strings.ContainsAny(addr, "\r\n") {
			return nil, validation("emails", "email")
		}
		if !seen[addr] {
			seen[addr] = true
			out = append(out, addr)
		}
	}
	if len(out) > maxRecipients {
		return nil, validation("emails", "max")
	}
	if _, err := s.d.Plants.ReplaceAlarmRecipients(ctx, sc, plantID, out); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}
