// Package alarms manages alarm rules (01 §7.12, 05 §10).
//
// Visibility is R213: the alarms table carries only company_id, so a
// building-scoped principal would otherwise see every rule in the company.
// This service intersects instead — a rule is readable when at least one of
// its analyzers is inside the caller's scope, which is what legacy did by
// filtering on accessible analyzer ids.
package alarms

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// AnalyzerRef is an attached analyzer, labelled for the list column "applied
// analyzers" (01 §7.12) so the screen needs no second request.
//
// LastReadingAt rides along because the data-communication evaluation (R216)
// needs exactly that and nothing else — no second read, no hypertable access.
type AnalyzerRef struct {
	ID                 uuid.UUID
	InstallationNumber string
	BuildingID         *uuid.UUID
	LastReadingAt      *time.Time
}

// Rule is one alarm with the attachments the API returns alongside it.
type Rule struct {
	Alarm     model.Alarm
	Analyzers []AnalyzerRef
	Channels  []model.AlarmChannel
}

// Input is a create or a full replace. The settings live on Alarm, and the
// service copies only the fields Type gives meaning to.
type Input struct {
	Name        string
	Type        model.AlarmType
	IsEnabled   bool
	Alarm       model.Alarm
	AnalyzerIDs []uuid.UUID
	Channels    []model.AlarmChannel
}

// Deps is what New needs.
type Deps struct {
	Alarms    store.AlarmRepository
	Analyzers store.AnalyzerRepository
	Clock     clock.Clock
}

// Service implements alarm rule management.
//
// ed is nil in a process that only does CRUD (the API before an evaluate is
// wired); WithEvaluate supplies it. A method reached without its own deps
// answers Unavailable rather than panicking on a nil interface.
type Service struct {
	d  Deps
	ed *EvaluateDeps
	nd *NotifyDeps
	rd *RunDeps
	dd *DispatchDeps
	// loc is Europe/Istanbul. NOT dto.Istanbul: internal/service may not import
	// internal/api (TestServiceLayerImportBoundaries), which is why
	// internal/service/billing loads its own the same way.
	loc *time.Location
}

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Alarms == nil || d.Analyzers == nil || d.Clock == nil {
		return nil, errors.New("alarms: Alarms, Analyzers and Clock are required")
	}
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return nil, errors.Join(errors.New("alarms: load Europe/Istanbul"), err)
	}
	return &Service{d: d, loc: loc}, nil
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

// notFound is R213's single answer for unknown, foreign and out-of-scope:
// store.ErrNotFound, which the API layer already renders as 404.
var notFound = store.ErrNotFound

// R231. The e-mail shape is deliberately loose — the SMTP server is the real
// judge — but it must be one @ with something either side and no whitespace.
var (
	emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
	phoneRe = regexp.MustCompile(`^\+?[0-9]{10,15}$`)
)

const (
	maxEmailTargets = 50
	maxSMSTargets   = 20
	maxNameLength   = 200
)

// settingsFor copies ONLY the fields Type gives meaning to, so a leftover from
// the dialog's type switch can never reach the row — and, for R212, so a
// voltage threshold cannot be stored even by a caller that got past Validate.
func settingsFor(in Input) model.Alarm {
	out := model.Alarm{
		Name: strings.TrimSpace(in.Name), Type: in.Type, IsEnabled: in.IsEnabled,
		NotificationFrequencyValue: in.Alarm.NotificationFrequencyValue,
		NotificationFrequencyUnit:  in.Alarm.NotificationFrequencyUnit,
	}
	switch in.Type {
	case model.AlarmTypeReactiveLimit:
		out.InductiveRatioThreshold = in.Alarm.InductiveRatioThreshold
		out.InductivePeriodValue, out.InductivePeriodUnit = in.Alarm.InductivePeriodValue, in.Alarm.InductivePeriodUnit
		out.CapacitiveRatioThreshold = in.Alarm.CapacitiveRatioThreshold
		out.CapacitivePeriodValue, out.CapacitivePeriodUnit = in.Alarm.CapacitivePeriodValue, in.Alarm.CapacitivePeriodUnit
		out.ActiveConsumptionMax = in.Alarm.ActiveConsumptionMax
		out.ActiveConsumptionMaxPeriodValue = in.Alarm.ActiveConsumptionMaxPeriodValue
		out.ActiveConsumptionMaxPeriodUnit = in.Alarm.ActiveConsumptionMaxPeriodUnit
		out.ActiveConsumptionMin = in.Alarm.ActiveConsumptionMin
		out.ActiveConsumptionMinPeriodValue = in.Alarm.ActiveConsumptionMinPeriodValue
		out.ActiveConsumptionMinPeriodUnit = in.Alarm.ActiveConsumptionMinPeriodUnit
	case model.AlarmTypeDataCommunication:
		out.CommunicationThresholdHours = in.Alarm.CommunicationThresholdHours
	case model.AlarmTypeCurrentVoltagePower:
		// R212: VoltageMax/VoltageMin are deliberately NOT copied.
		out.PowerMax, out.PowerMin = in.Alarm.PowerMax, in.Alarm.PowerMin
	case model.AlarmTypeInvoiceIncrease:
		out.InvoiceThresholdPct = in.Alarm.InvoiceThresholdPct
	}
	return out
}

// normaliseChannels validates, lower-cases, de-duplicates and caps (R231).
// The primary key (alarm_id, channel, target) already forbids exact
// duplicates; de-duplicating here turns a user's repeated address into a
// no-op rather than an insert conflict.
func normaliseChannels(in []model.AlarmChannel) ([]model.AlarmChannel, error) {
	seen := map[string]bool{}
	out := make([]model.AlarmChannel, 0, len(in))
	counts := map[model.NotifyChannel]int{}
	for _, c := range in {
		target := strings.TrimSpace(c.Target)
		switch c.Channel {
		case model.NotifyChannelEmail:
			if len(target) > 254 || !emailRe.MatchString(target) {
				return nil, validation("channels", "email")
			}
			target = strings.ToLower(target)
		case model.NotifyChannelSMS:
			// R211: stored so a provider can be wired later. Nothing is sent.
			if !phoneRe.MatchString(target) {
				return nil, validation("channels", "phone")
			}
		default:
			return nil, validation("channels", "oneof")
		}
		key := string(c.Channel) + "|" + target
		if seen[key] {
			continue
		}
		seen[key] = true
		counts[c.Channel]++
		out = append(out, model.AlarmChannel{Channel: c.Channel, Target: target})
	}
	if counts[model.NotifyChannelEmail] > maxEmailTargets || counts[model.NotifyChannelSMS] > maxSMSTargets {
		return nil, validation("channels", "max")
	}
	return out, nil
}

// visibleAnalyzers is R213. The analyzer repository applies the Scope's
// building branch, so what comes back IS the intersection; an empty result for
// a scope that does not span the company means the rule is not this
// principal's business.
func (s *Service) visibleAnalyzers(ctx context.Context, sc store.Scope, alarmID uuid.UUID) ([]AnalyzerRef, error) {
	attached, err := s.d.Alarms.Analyzers(ctx, sc, alarmID) // already company-scoped
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(attached))
	for _, a := range attached {
		ids = append(ids, a.AnalyzerID)
	}
	var refs []AnalyzerRef
	if len(ids) > 0 {
		rows, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{IDs: ids, Page: store.Page{Limit: int32(len(ids))}})
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			refs = append(refs, AnalyzerRef{ID: r.ID, InstallationNumber: r.InstallationNumber,
				BuildingID: r.BuildingID, LastReadingAt: r.LastReadingAt})
		}
	}
	if len(refs) == 0 && !sc.AllBuildings {
		return nil, notFound
	}
	return refs, nil
}

// load assembles a Rule, enforcing R213 on the way.
func (s *Service) load(ctx context.Context, sc store.Scope, a model.Alarm) (Rule, error) {
	refs, err := s.visibleAnalyzers(ctx, sc, a.ID)
	if err != nil {
		return Rule{}, err
	}
	channels, err := s.d.Alarms.Channels(ctx, sc, a.ID)
	if err != nil {
		return Rule{}, err
	}
	return Rule{Alarm: a, Analyzers: refs, Channels: channels}, nil
}

// Get returns one rule, or ErrNotFound when it is outside the scope (R213).
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (Rule, error) {
	a, err := s.d.Alarms.Get(ctx, sc, id)
	if err != nil {
		return Rule{}, err
	}
	return s.load(ctx, sc, a)
}

// List returns the rules the caller may see. A rule the scope cannot reach is
// skipped rather than failing the listing.
func (s *Service) List(ctx context.Context, sc store.Scope, f store.AlarmFilter) ([]Rule, error) {
	rows, err := s.d.Alarms.List(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, a := range rows {
		rule, err := s.load(ctx, sc, a)
		if errors.Is(err, store.ErrNotFound) {
			continue // R213: not this principal's rule
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

// prepare runs the shared create/update validation.
func prepare(in Input) (model.Alarm, []model.AlarmChannel, error) {
	if name := strings.TrimSpace(in.Name); name == "" || len(name) > maxNameLength {
		return model.Alarm{}, nil, validation("name", "required")
	}
	// Validate the INCOMING settings, not the stripped row: settingsFor drops
	// the fields another type owns, so validating its output could never see a
	// voltage threshold and R212's refusal would never fire.
	incoming := in.Alarm
	incoming.Type, incoming.Name = in.Type, strings.TrimSpace(in.Name)
	if err := alarm.Validate(incoming, len(in.AnalyzerIDs)); err != nil {
		var ve *alarm.ValidationError
		if errors.As(err, &ve) {
			params := make(map[string]any, len(ve.Fields))
			for field, codes := range ve.Fields {
				params[field] = codes
			}
			return model.Alarm{}, nil, perr.Validation.WithParams(params)
		}
		return model.Alarm{}, nil, err
	}
	channels, err := normaliseChannels(in.Channels)
	if err != nil {
		return model.Alarm{}, nil, err
	}
	return settingsFor(in), channels, nil
}

// Create stores a rule and its attachments.
//
// The analyzer ids are NOT pre-filtered: ReplaceAnalyzers refuses the whole
// call with ErrNotFound when any of them is outside the scope, which is the
// check that must decide, because it is the one the database enforces.
func (s *Service) Create(ctx context.Context, sc store.Scope, in Input) (Rule, error) {
	row, channels, err := prepare(in)
	if err != nil {
		return Rule{}, err
	}
	now := s.d.Clock.Now()
	row.CompanyID, row.CreatedAt, row.UpdatedAt = sc.CompanyID, now, now

	created, err := s.d.Alarms.Create(ctx, sc, row)
	if err != nil {
		return Rule{}, err
	}
	if err := s.d.Alarms.ReplaceAnalyzers(ctx, sc, created.ID, in.AnalyzerIDs); err != nil {
		return Rule{}, err
	}
	if err := s.d.Alarms.ReplaceChannels(ctx, sc, created.ID, channels); err != nil {
		return Rule{}, err
	}
	return s.load(ctx, sc, created)
}

// Update is a FULL replace of the mutable fields: a threshold left out is
// cleared, which is what makes "remove this limit" expressible at all.
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (Rule, error) {
	current, err := s.Get(ctx, sc, id) // R213 first: no write to a rule we cannot see
	if err != nil {
		return Rule{}, err
	}
	row, channels, err := prepare(in)
	if err != nil {
		return Rule{}, err
	}
	row.ID, row.CompanyID = current.Alarm.ID, current.Alarm.CompanyID
	row.CreatedAt, row.UpdatedAt = current.Alarm.CreatedAt, s.d.Clock.Now()

	updated, err := s.d.Alarms.Update(ctx, sc, row)
	if err != nil {
		return Rule{}, err
	}
	if err := s.d.Alarms.ReplaceAnalyzers(ctx, sc, updated.ID, in.AnalyzerIDs); err != nil {
		return Rule{}, err
	}
	if err := s.d.Alarms.ReplaceChannels(ctx, sc, updated.ID, channels); err != nil {
		return Rule{}, err
	}
	return s.load(ctx, sc, updated)
}

// Delete soft-deletes a rule the caller may see.
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if _, err := s.Get(ctx, sc, id); err != nil {
		return err
	}
	return s.d.Alarms.SoftDelete(ctx, sc, id, s.d.Clock.Now())
}

// Events returns a rule's firing history, R213 first.
func (s *Service) Events(ctx context.Context, sc store.Scope, id uuid.UUID, f store.AlarmEventFilter) ([]model.AlarmEvent, error) {
	if _, err := s.Get(ctx, sc, id); err != nil {
		return nil, err
	}
	f.AlarmID = &id
	return s.d.Alarms.ListEvents(ctx, sc, f)
}
