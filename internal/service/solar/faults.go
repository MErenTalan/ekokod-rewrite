package solar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const (
	faultWindow    = 7 * 24 * time.Hour   // R286: the fetch window
	faultRetention = 180 * 24 * time.Hour // R287: ≫ the window, so nothing is resent
	alarmCategory  = "isolar-alarm"
)

// Legacy cron labels (alarm-check/route.ts): fault_type and fault_level.
var (
	faultTypeLabels  = map[int32]string{1: "Arıza", 2: "Alarm", 3: "Bildirim", 4: "Öneri"}
	faultLevelLabels = map[int32]string{1: "Kritik", 2: "Önemli", 3: "Minör", 4: "Uyarı"}
)

func label(m map[int32]string, v *int32) string {
	if v != nil {
		if s, ok := m[*v]; ok {
			return s
		}
	}
	return "Bilinmeyen"
}

// FaultSentence is the Turkish fault line body (legacy translateAlarmMessage).
func FaultSentence(f model.PlantFault) string {
	device := "Bu inverter"
	if f.DeviceName != nil && strings.TrimSpace(*f.DeviceName) != "" {
		device = strings.TrimSpace(*f.DeviceName)
	}
	if text, ok := TranslateFault(f.Name); ok {
		return fmt.Sprintf("%s cihazında %s hatası oluştu.", device, text)
	}
	return fmt.Sprintf("%s cihazında hata: %s", device, f.Name)
}

// FetchAlarms is isolar.fetch_alarms: per company, fetch the last 7 days of
// faults per credential, store them on that company's plants, forward the
// new ones once (R286, R287) and prune. One company's failure never stops another's.
func (s *Service) FetchAlarms(ctx context.Context) error {
	linked, err := s.d.AdminSolar.LinkedPlants(ctx)
	if err != nil {
		return err
	}
	seen := map[uuid.UUID]bool{}
	for _, p := range linked {
		if seen[p.CompanyID] {
			continue
		}
		seen[p.CompanyID] = true
		s.fetchCompanyAlarms(ctx, p.CompanyID)
	}
	return nil
}

func (s *Service) fetchCompanyAlarms(ctx context.Context, companyID uuid.UUID) {
	sc := store.SystemScope(companyID)
	now := s.d.Clock.Now()
	scope, _ := json.Marshal(map[string]any{"company_id": companyID})
	run, runErr := s.d.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &companyID, JobType: job.TypeSolarFetchAlarms, Scope: scope, StartedAt: now, Status: "running"})
	var processed, failed int32

	plants, err := s.d.Plants.ListLinked(ctx, sc)
	if err != nil {
		failed++
	}
	byCredential := map[uuid.UUID][]model.PowerPlant{}
	for _, p := range plants {
		if p.IsolarCredentialID != nil {
			byCredential[*p.IsolarCredentialID] = append(byCredential[*p.IsolarCredentialID], p)
		}
	}
	for credID, group := range byCredential {
		if err := s.storeCredentialFaults(ctx, sc, credID, group, now); err != nil {
			failed++
			s.alarmMessage(ctx, sc, "error", "iSolar arıza kayıtları alınamadı ("+classify(err).code+").")
			continue
		}
		for _, p := range group {
			if err := s.forwardPlant(ctx, sc, p, now); err != nil {
				failed++
				continue
			}
			processed++
		}
	}
	if _, err := s.d.Faults.Prune(ctx, sc, now.Add(-faultRetention)); err != nil {
		failed++
	}
	if runErr == nil {
		status := "success"
		if failed > 0 {
			status = "partial"
		}
		_, _ = s.d.Ops.FinishRun(ctx, sc, run.ID, status, processed, 0, failed, nil, nil, s.d.Clock.Now())
	}
}

// storeCredentialFaults fetches one credential's account-wide faults and
// keeps only those naming one of this company's plants.
func (s *Service) storeCredentialFaults(ctx context.Context, sc store.Scope, credID uuid.UUID, plants []model.PowerPlant, now time.Time) error {
	creds, err := s.d.Creds.Open(ctx, sc, credID)
	if err != nil {
		return err
	}
	faults, err := s.d.ISolar.Faults(ctx, creds, now.Add(-faultWindow), now)
	if err != nil {
		return err
	}
	byPs := map[string]uuid.UUID{}
	for _, p := range plants {
		byPs[*p.IsolarPsID] = p.ID
	}
	var rows []model.PlantFault
	for _, f := range faults {
		plantID, ok := byPs[f.PSID]
		if !ok {
			continue
		}
		rows = append(rows, model.PlantFault{PlantID: plantID, Ref: f.Ref, Code: f.Code, Name: f.Message, Level: f.Level, Type: f.Type,
			DeviceName: f.DeviceName, OccurredAt: f.OccurredAt, ClosedAt: f.ClosedAt, FirstSeenAt: now})
	}
	_, err = s.d.Faults.Upsert(ctx, sc, rows)
	return err
}

// forwardPlant claims the plant's unforwarded faults, mails them once, and
// releases the claims when the mail does not go out (R287).
func (s *Service) forwardPlant(ctx context.Context, sc store.Scope, plant model.PowerPlant, now time.Time) error {
	recipients, err := s.d.Plants.AlarmRecipients(ctx, sc, plant.ID)
	if err != nil || len(recipients) == 0 {
		return err
	}
	open, err := s.d.Faults.Unforwarded(ctx, sc, plant.ID, now.Add(-faultWindow))
	if err != nil || len(open) == 0 {
		return err
	}
	refs := make([]string, len(open))
	for i, f := range open {
		refs[i] = f.Ref
	}
	claimed, err := s.d.Faults.Claim(ctx, sc, plant.ID, refs)
	if err != nil || len(claimed) == 0 {
		return err
	}
	mine := map[string]bool{}
	for _, r := range claimed {
		mine[r] = true
	}
	var lines []string
	for _, f := range open {
		if mine[f.Ref] {
			lines = append(lines, fmt.Sprintf("[%s - %s] %s | Zaman: %s", label(faultTypeLabels, f.Type), label(faultLevelLabels, f.Level),
				FaultSentence(f), f.OccurredAt.In(istanbul).Format("02.01.2006 15:04")))
		}
	}
	name := plant.Name
	if plant.IsolarPsName != nil && *plant.IsolarPsName != "" {
		name = *plant.IsolarPsName
	}
	emails := make([]string, len(recipients))
	for i, r := range recipients {
		emails[i] = r.Email
	}
	msg := mail.Message{To: emails, Subject: fmt.Sprintf("iSolar Alarm Bildirimi: %s - %d yeni alarm", name, len(lines)),
		Text: name + " santralinde yeni iSolar alarmları:\n\n" + strings.Join(lines, "\n") + "\n"}

	reason, hide, sendErr := s.send(ctx, sc, msg)
	if sendErr != nil {
		if err := s.d.Faults.Release(ctx, sc, plant.ID, claimed); err != nil {
			return err
		}
		s.alarmMessage(ctx, sc, "error", fmt.Sprintf("%s santralinin %d alarmı iletilemedi: %s", name, len(lines),
			secret.Redact(reason+": "+sendErr.Error(), hide)))
		return nil
	}
	s.alarmMessage(ctx, sc, "info", fmt.Sprintf("%s santralinin %d yeni alarmı %d alıcıya iletildi.", name, len(lines), len(emails)))
	return nil
}

func (s *Service) send(ctx context.Context, sc store.Scope, msg mail.Message) (reason string, hide []string, err error) {
	if s.d.SMTP == nil || s.d.Mail == nil {
		return "şirketin SMTP ayarları yok", nil, errors.New("mail not configured")
	}
	settings, err := s.d.SMTP.Get(ctx, sc)
	if err != nil {
		return "şirketin SMTP ayarları yok", nil, err
	}
	password, err := s.d.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return "SMTP parolası okunamadı", nil, err
	}
	hide = secret.Fragments(string(password))
	if err := s.d.Mail.Send(ctx, settings, password, msg); err != nil {
		return "e-posta gönderilemedi", hide, err
	}
	return "", nil, nil
}

func (s *Service) alarmMessage(ctx context.Context, sc store.Scope, status, text string) {
	companyID := sc.CompanyID
	_, _ = s.d.Ops.AppendMessage(ctx, sc, model.OperationalMessage{CompanyID: &companyID, Kind: "alarm", Category: alarmCategory,
		Status: status, Message: text, CreatedAt: s.d.Clock.Now()})
}
