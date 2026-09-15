// Package credentials is the write-only credential service: it stores each
// company's integration credentials sealed at rest (F1's
// store.IntegrationRepository does the actual sealing/opening), and drives
// iSolarCloud's OAuth flow (authorize URL, code exchange, locked token
// refresh) on top of it.
//
// "Write-only" means: no exported method here returns plaintext secret
// material except Open (which builds an integration.Credentials for an
// adapter call — that is the one legitimate internal consumer) and
// ISolarAccessToken (which returns exactly one integration.Secret, the
// bearer token an isolar call needs — never the whole credential). Every
// other read (List, Configure's and Update's return values) is a View,
// which carries only a HasSecret bool and ExtraKeys names — see view.go's
// doc comment and TestViewCarriesNoSecretMaterial.
package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/shopspring/decimal"
)

// ErrInvalidState is returned by ISolarCallback for a bad, expired,
// wrong-key or already-consumed (replayed) OAuth state.
var ErrInvalidState = errors.New("credentials: invalid oauth state")

// ErrInvalidSettings is returned when Input.Settings carries an unknown key
// or a key that looks like it is trying to smuggle a secret through the one
// JSON column the API returns verbatim. See settings.go.
var ErrInvalidSettings = errors.New("credentials: invalid settings")

// ErrInvalidExtraKey is returned when an Input.Extra key does not match
// extraKeyPattern (fix round 1, folded minor "invalid key accepted"). Extra
// key names end up as map keys the rest of this package (and isolar.Client,
// which reads creds.Extra["access_token"]/["refresh_token"] by exact name)
// trusts implicitly; an unvalidated name is how an operator-facing field
// could smuggle something unexpected into that trust.
var ErrInvalidExtraKey = errors.New("credentials: invalid extra key")

// extraKeyPattern is the allowed shape for an Input.Extra key: lowercase
// ASCII, starting with a letter, snake_case.
var extraKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// validateExtraKeys rejects any Input.Extra key that does not match
// extraKeyPattern. Checked before ANY Extra key reaches mergeExtra, on both
// Configure and Update.
func validateExtraKeys(delta map[string]integration.Secret) error {
	for k := range delta {
		if !extraKeyPattern.MatchString(k) {
			return fmt.Errorf("%w: %q", ErrInvalidExtraKey, k)
		}
	}
	return nil
}

// minStateKeyLen is New's minimum for Deps.StateKey: an HMAC key shorter
// than its hash's block-relevant strength defeats the point of signing the
// OAuth state at all. 32 bytes matches Deps.StateKey's own doc comment
// (HMAC(JWTSigningKey, …)) and internal/platform/crypto's cipher key size.
const minStateKeyLen = 32

// isolarTokenLockKey is the Deps.Locker critical-section key EVERY
// read-merge-write of an isolar credential's Extra blob (or a read of its
// currently-fresh access token) serialises under: ISolarAccessToken's
// refresh, ISolarCallback's persistIsolarToken, and Update's own Extra
// merge for an isolar credential (I3) — same key everywhere so all three
// exclude each other. Companies never share a lock: two different isolar
// credentials belonging to the SAME company would still (harmlessly)
// serialise through this key, since the key only names sc.CompanyID; that
// is a company only ever having one isolar credential in practice
// (integration_credentials(company_id, definition_id) unique, and isolar
// has exactly one definition per region), not a correctness requirement.
func isolarTokenLockKey(companyID uuid.UUID) string {
	return "isolar:token:" + companyID.String()
}

// releaseLeaseTimeout bounds releaseLease's own detached context (folded
// minor: "lease.Release uses context.WithoutCancel(ctx) with a short
// timeout").
const releaseLeaseTimeout = 5 * time.Second

// releaseLease releases lease on a context DERIVED from ctx's values but
// DETACHED from its cancellation (context.WithoutCancel), bounded by its
// own short timeout. Release must still run — and must not hang — even
// when ctx itself has just been cancelled or has already deadlined: the
// critical section it is closing out already happened, so an early
// cancellation must not skip cleanup, and a wedged Locker backend must not
// hang the caller forever either.
func releaseLease(ctx context.Context, lease lock.Lease) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseLeaseTimeout)
	defer cancel()
	_ = lease.Release(rctx)
}

// errNotIsolar is returned by the iSolar-only methods when id names a
// credential for a different provider. It is deliberately unexported and
// UNwrapped by any sentinel: callers of ISolarAuthorizeURL/ISolarCallback/
// ISolarAccessToken on a non-isolar credential get a plain error, not a
// mis-typed ErrInvalidState or ErrNotFound.
var errNotIsolar = errors.New("credentials: not an isolar credential")

// isolarRefreshWindow is 06 §6 step 4's "refreshes when expiring within
// 5 min".
const isolarRefreshWindow = 5 * time.Minute

// isolarTokenLockTTL is the Provider-defaults table's generic "lock TTL is
// 60 s".
const isolarTokenLockTTL = 60 * time.Second

// isolarStateCallbackTimeout bounds ONLY the single-use nonce Consume call
// inside ISolarCallback (R3): independent of the caller's ctx, so a broken
// Consumer backend cannot turn a replayed callback into a hang.
const isolarStateCallbackTimeout = 2 * time.Second

// View is the ONLY read model of a credential. See view.go.

// Input is Configure/Update's write model. See service.go's Configure/Update
// doc comments for the exact PATCH semantics of each optional field.
type Input struct {
	Provider model.IntegrationProvider
	Subtype  string
	Username *string
	// Secret is nil on Update to mean "leave the stored secret unchanged"
	// (05 §15 PATCH). On Configure a nil Secret stores no secret at all —
	// integration_credentials.secret_enc has no NOT NULL constraint, unlike
	// smtp_settings.password_enc, so some providers (pm5340, before an
	// operator supplies one) may have none yet.
	Secret *integration.Secret
	// Extra is nil on Update to mean "leave every Extra key unchanged". A
	// non-nil map (including an empty one) is merged key by key into
	// whatever is already stored: a key with a non-zero Secret is set, a
	// key with a zero Secret (IsZero() == true) is DELETED. On Configure,
	// Extra simply becomes the initial key set (a zero Secret in it is a
	// no-op, since there is nothing to delete yet).
	Extra              map[string]integration.Secret
	Settings           json.RawMessage
	PM5340URL          *string
	InstallationNumber *string // pm5340 only: creates/updates its single analyzer
	IsolarRegion       *string
	IsActive           *bool
}

// BackfillInput is Service.Backfill's write model — see job.BackfillPayload,
// which it is turned into verbatim.
type BackfillInput struct {
	AnalyzerIDs []uuid.UUID
	Kinds       []model.ReadingKind
	From, To    time.Time
	// Force is R53's re-run escape hatch, threaded straight through to
	// job.BackfillPayload.Force: see its doc. False (the default) keeps the
	// resumable, plain-id behaviour every existing caller already gets.
	Force bool
}

// Verifier proves one set of credentials can authenticate against its
// provider. Satisfied by *integration.Registry's adapters (their Verify
// method — 06 §1's MeterDataSource) and, for isolar, a shim Task 16 builds
// over *isolar.Client.Verify.
type Verifier interface {
	Verify(ctx context.Context, creds integration.Credentials) error
}

// VerifierResolver looks up the Verifier for a provider. Task 16 builds the
// production implementation over *integration.Registry plus the isolar
// shim; this package only ever consumes the interface.
type VerifierResolver interface {
	Verifier(p integration.Provider) (Verifier, error)
}

// ISolarTokens is the slice of *isolar.Client this package drives the OAuth
// flow through. credentials imports internal/integration/isolar for
// isolar.Token ONLY — never for the client's own concrete type or its other
// methods (Plants/Devices/Faults/*Series belong to Task 13's ingest side).
type ISolarTokens interface {
	AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error)
	ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (isolar.Token, error)
	Refresh(ctx context.Context, creds integration.Credentials) (isolar.Token, error)
}

// Deps is everything Service needs.
type Deps struct {
	Integrations store.IntegrationRepository
	Analyzers    store.AnalyzerRepository
	Verifiers    VerifierResolver
	ISolar       ISolarTokens
	Enqueuer     ingest.Enqueuer // *job.Client

	// Locker is used ONLY for ISolarAccessToken's refresh critical section
	// (R2): a genuine, bounded, always-released mutual-exclusion lock, the
	// canonical type from internal/platform/lock — never httpx.Locker,
	// which this package has no other reason to depend on.
	Locker lock.Locker
	// Nonces is the OAuth state nonce's single-use, non-blocking consume
	// (R3) — NEVER Locker: see ISolarCallback's doc comment.
	Nonces lock.Consumer

	Clock clock.Clock
	// StateKey is HMAC(JWTSigningKey, "isolar-oauth-state/v1"), derived by
	// Task 16.
	StateKey []byte
	// RedirectURI is EKOKOD_ISOLAR_REDIRECT_URL or
	// PublicURL+"/integrations/isolar/callback".
	RedirectURI string
	MaxRetry    int
}

// Service is the write-only credential service. It satisfies
// ingest.CredentialOpener structurally via Open.
type Service struct {
	deps Deps
}

// New validates deps and returns a Service. Every field is required except
// where noted.
func New(d Deps) (*Service, error) {
	switch {
	case d.Integrations == nil:
		return nil, fmt.Errorf("credentials: Deps.Integrations is required")
	case d.Analyzers == nil:
		return nil, fmt.Errorf("credentials: Deps.Analyzers is required")
	case d.Verifiers == nil:
		return nil, fmt.Errorf("credentials: Deps.Verifiers is required")
	case d.ISolar == nil:
		return nil, fmt.Errorf("credentials: Deps.ISolar is required")
	case d.Enqueuer == nil:
		return nil, fmt.Errorf("credentials: Deps.Enqueuer is required")
	case d.Locker == nil:
		return nil, fmt.Errorf("credentials: Deps.Locker is required")
	case d.Nonces == nil:
		return nil, fmt.Errorf("credentials: Deps.Nonces is required")
	case d.Clock == nil:
		return nil, fmt.Errorf("credentials: Deps.Clock is required")
	case len(d.StateKey) < minStateKeyLen:
		return nil, fmt.Errorf("credentials: Deps.StateKey must be at least %d bytes", minStateKeyLen)
	case d.RedirectURI == "":
		return nil, fmt.Errorf("credentials: Deps.RedirectURI is required")
	}
	return &Service{deps: d}, nil
}

// getCredential finds sc's credential id and resolves its definition. It IS
// the tenancy check: ListCredentials only ever returns sc.CompanyID's rows,
// so an id belonging to another company simply never appears and this
// returns store.ErrNotFound — never a cross-tenant read (TestOpenIsScoped).
func (s *Service) getCredential(ctx context.Context, sc store.Scope, id uuid.UUID) (model.IntegrationCredential, model.IntegrationDefinition, error) {
	if !sc.Valid() {
		return model.IntegrationCredential{}, model.IntegrationDefinition{}, store.ErrInvalidScope
	}
	creds, err := s.deps.Integrations.ListCredentials(ctx, sc)
	if err != nil {
		return model.IntegrationCredential{}, model.IntegrationDefinition{}, err
	}
	for _, c := range creds {
		if c.ID == id {
			def, derr := s.definitionByID(ctx, sc, c.DefinitionID)
			if derr != nil {
				return model.IntegrationCredential{}, model.IntegrationDefinition{}, derr
			}
			return c, def, nil
		}
	}
	return model.IntegrationCredential{}, model.IntegrationDefinition{}, store.ErrNotFound
}

// definitionByID resolves a definition id against the (small, platform-wide)
// catalogue. Used because IntegrationRepository has no "get definition by
// id" method — only by (provider, subtype), which a credential row does not
// carry directly.
func (s *Service) definitionByID(ctx context.Context, sc store.Scope, id uuid.UUID) (model.IntegrationDefinition, error) {
	defs, err := s.deps.Integrations.Definitions(ctx, sc)
	if err != nil {
		return model.IntegrationDefinition{}, err
	}
	for _, d := range defs {
		if d.ID == id {
			return d, nil
		}
	}
	return model.IntegrationDefinition{}, store.ErrNotFound
}

// redactedText renders err's text with every secret fragment creds carries
// replaced — 03 §7: a credential must never reach an operator-facing error.
// Modelled on internal/ingest/report.go's redacted().
func redactedText(creds integration.Credentials, err error) string {
	if err == nil {
		return ""
	}
	return secret.Redact(err.Error(), creds.Fragments())
}

// redactedVerifyError wraps a verifier's error for Verify (fix round 2, M
// dropped last round): Error() renders the secret-redacted text (03 §7 —
// a credential must never reach an operator-facing error), but Unwrap()
// returns the ORIGINAL, unredacted verifier error, so the error CHAIN is
// preserved — a caller can still errors.Is(err, integration.ErrAuth) (or
// errors.As into an *integration.Error) to distinguish auth failures from
// other verify errors, exactly as if this wrapper were not there. Fix
// round 1 returned errors.New(redactedText(...)), which threw the chain
// away entirely: errors.Is/As against the original sentinel always
// returned false, even though the redacted TEXT was correct.
type redactedVerifyError struct {
	text string
	err  error
}

func (e *redactedVerifyError) Error() string { return e.text }
func (e *redactedVerifyError) Unwrap() error { return e.err }

// zeroBytes overwrites b in place, best effort — the plaintext buffers this
// package builds (a Secret's Reveal() copy, a merged Extra JSON blob) are
// zeroed once sealed. This is best effort, not a guarantee: Go's garbage
// collector may have already copied the backing array (e.g. during a slice
// append or a stack-to-heap move) before this runs, and neither this
// package nor the runtime can find every such copy. Documented here rather
// than claimed as a real security boundary.
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// zeroExtraPlain clears every plaintext value in m to "" (fix round 1: its
// earlier doc comment claimed it "zeros" the plaintext, which was never
// true for a map[string]string — Go strings are immutable, so
// []byte(v) copies v's bytes into a NEW slice; zeroing that copy, as the
// old code did, touches nothing but a value that was about to be discarded
// anyway, leaving the original string's backing bytes untouched in memory
// for as long as the GC happens to keep them alive). This function's real
// effect is narrower than "zeroing": it only ensures m itself no longer
// holds the plaintext values once this returns, so a later read of m (or a
// dump of it) cannot recover them from the map — it makes no claim about,
// and cannot deliver, wiping the immutable string bytes those values were
// copied from. Callers needing an actual best-effort wipe use zeroBytes on
// a real []byte, as service.go's secretPlain/extraJSON already do.
func zeroExtraPlain(m map[string]string) {
	for k := range m {
		m[k] = ""
	}
}

// validatePM5340URL is R28: scheme http/https and a non-empty host. M5
// (final-review-A): userinfo (http://user:pass@host) is refused outright —
// pm5340_url is stored in plaintext (never sealed, unlike Secret/Extra) and
// returned verbatim by the write-only View (view.go), so a credential
// embedded in it would be stored unsealed and handed straight back to any
// caller of View, defeating this whole package's write-only design.
func validatePM5340URL(raw *string) error {
	if raw == nil {
		return nil
	}
	u, err := url.Parse(*raw)
	if err != nil {
		return fmt.Errorf("%w: pm5340_url is not a valid URL", ErrInvalidSettings)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: pm5340_url must have scheme http or https", ErrInvalidSettings)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: pm5340_url must have a host", ErrInvalidSettings)
	}
	if u.User != nil {
		return fmt.Errorf("%w: pm5340_url must not contain userinfo", ErrInvalidSettings)
	}
	return nil
}

// mergeExtra applies delta on top of base (base may be nil), returning a
// NEW map: a key with a non-zero Secret is set/overwritten; a key with a
// zero Secret (Secret{}.IsZero() == true) is deleted. base is never
// mutated.
func mergeExtra(base map[string]string, delta map[string]integration.Secret) map[string]string {
	out := make(map[string]string, len(base)+len(delta))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range delta {
		if v.IsZero() {
			delete(out, k)
			continue
		}
		out[k] = v.Reveal()
	}
	return out
}

// Configure creates a new credential for (sc.CompanyID, in.Provider,
// in.Subtype) — integration_credentials(company_id, definition_id) is
// unique, so there is at most one credential per definition per company.
func (s *Service) Configure(ctx context.Context, sc store.Scope, in Input) (View, error) {
	if !sc.Valid() {
		return View{}, store.ErrInvalidScope
	}
	if !in.Provider.Valid() {
		return View{}, fmt.Errorf("credentials: %q is not a valid provider", in.Provider)
	}
	settingsJSON, err := validateSettings(in.Settings)
	if err != nil {
		return View{}, err
	}
	if err := validatePM5340URL(in.PM5340URL); err != nil {
		return View{}, err
	}
	if err := validateExtraKeys(in.Extra); err != nil {
		return View{}, err
	}

	def, err := s.deps.Integrations.Definition(ctx, sc, in.Provider, in.Subtype)
	if err != nil {
		return View{}, err
	}

	// R45 (fix round 1, I2): a credential already exists for this
	// (company, definition) — integration_credentials(company_id,
	// definition_id) is unique, so UpsertCredential below would silently
	// REPLACE it: wipe its sealed Extra (any OAuth tokens it carries) and
	// reset settings/is_active to whatever this fresh Input carries (which,
	// for a bare "reconfigure" call, is typically all zero values). Refuse
	// instead; callers that mean to change an existing credential use
	// Update, which read-merges every field.
	if _, cerr := s.deps.Integrations.Credential(ctx, sc, def.ID); cerr == nil {
		return View{}, fmt.Errorf("%w: a credential already exists for %s/%s (use Update)", store.ErrConflict, in.Provider, in.Subtype)
	} else if !errors.Is(cerr, store.ErrNotFound) {
		return View{}, cerr
	}

	isActive := true
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	extraPlain := mergeExtra(nil, in.Extra)
	extraJSON, err := json.Marshal(extraPlain)
	if err != nil {
		return View{}, fmt.Errorf("credentials: encode extra: %w", err)
	}
	if len(extraPlain) == 0 {
		extraJSON = nil // nothing to seal; UpsertCredential leaves extra_enc NULL
	}

	var secretPlain []byte
	if in.Secret != nil {
		secretPlain = []byte(in.Secret.Reveal())
	}

	cred := model.IntegrationCredential{
		ID: uuid.New(), CompanyID: sc.CompanyID, DefinitionID: def.ID,
		Username: in.Username, Settings: settingsJSON,
		Pm5340URL: in.PM5340URL, IsolarRegion: in.IsolarRegion, IsActive: isActive,
	}
	saved, err := s.deps.Integrations.UpsertCredential(ctx, sc, cred, secretPlain, extraJSON)
	zeroBytes(secretPlain)
	zeroBytes(extraJSON)
	zeroExtraPlain(extraPlain)
	if err != nil {
		return View{}, err
	}

	if def.Provider == model.IntegrationProviderPM5340 && in.InstallationNumber != nil {
		if err := s.upsertPM5340Analyzer(ctx, sc, def, *in.InstallationNumber); err != nil {
			return View{}, err
		}
	}

	return s.viewOf(ctx, sc, saved, def)
}

// Update is a PATCH: every Input field left at its zero value (nil pointer,
// nil map, empty json.RawMessage) leaves the stored value UNCHANGED, except
// Secret and Extra, whose own nil semantics are documented on Input.
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (View, error) {
	if !sc.Valid() {
		return View{}, store.ErrInvalidScope
	}
	existing, def, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	if err := validateExtraKeys(in.Extra); err != nil {
		return View{}, err
	}

	settingsJSON := existing.Settings
	if len(in.Settings) > 0 {
		settingsJSON, err = validateSettings(in.Settings)
		if err != nil {
			return View{}, err
		}
	}

	pm5340URL := existing.Pm5340URL
	if in.PM5340URL != nil {
		if err := validatePM5340URL(in.PM5340URL); err != nil {
			return View{}, err
		}
		pm5340URL = in.PM5340URL
	}

	username := existing.Username
	if in.Username != nil {
		username = in.Username
	}
	isolarRegion := existing.IsolarRegion
	if in.IsolarRegion != nil {
		isolarRegion = in.IsolarRegion
	}
	isActive := existing.IsActive
	if in.IsActive != nil {
		isActive = *in.IsActive
	}

	var secretPlain []byte
	if in.Secret != nil {
		secretPlain = []byte(in.Secret.Reveal())
	}

	var extraJSON []byte
	var extraPlainForZero map[string]string
	if in.Extra != nil {
		mergeExtraNow := func() error {
			_, existingExtra, oerr := s.deps.Integrations.OpenSecret(ctx, sc, id)
			if oerr != nil {
				return oerr
			}
			base := map[string]string{}
			if len(existingExtra) > 0 {
				if uerr := json.Unmarshal(existingExtra, &base); uerr != nil {
					zeroBytes(existingExtra)
					return fmt.Errorf("credentials: decode stored extra: %w", uerr)
				}
			}
			zeroBytes(existingExtra)
			merged := mergeExtra(base, in.Extra)
			zeroExtraPlain(base)
			extraPlainForZero = merged
			var jerr error
			extraJSON, jerr = json.Marshal(merged)
			if jerr != nil {
				return fmt.Errorf("credentials: encode extra: %w", jerr)
			}
			return nil
		}

		if def.Provider == model.IntegrationProviderISolar {
			// I3 (fix round 2): the lease MUST stay held across
			// UpsertCredential's write below, not just across
			// mergeExtraNow's read-merge here — releasing right after
			// mergeExtraNow (fix round 1's mistake) left the window
			// between release and UpsertCredential completely unlocked,
			// so a concurrent ISolarAccessToken refresh (or
			// ISolarCallback) could read-merge-write in that gap and have
			// its own write silently clobbered by this Update's later
			// UpsertCredential, or vice versa — most dangerously the
			// refresh's brand-new refresh_token, since iSolarCloud
			// invalidates the previous one on every refresh, permanently
			// stranding the credential. defer (not an immediate release)
			// holds the lease across the re-read → merge → UpsertCredential
			// sequence below, exactly like ISolarCallback's own
			// Acquire/defer releaseLease pairing.
			lease, lerr := s.deps.Locker.Acquire(ctx, isolarTokenLockKey(sc.CompanyID), isolarTokenLockTTL)
			if lerr != nil {
				return View{}, lerr
			}
			defer releaseLease(ctx, lease)
			err = mergeExtraNow()
		} else {
			err = mergeExtraNow()
		}
		if err != nil {
			return View{}, err
		}
	}

	cred := model.IntegrationCredential{
		ID: id, CompanyID: sc.CompanyID, DefinitionID: existing.DefinitionID,
		Username: username, Settings: settingsJSON,
		Pm5340URL: pm5340URL, IsolarRegion: isolarRegion, IsActive: isActive,
	}
	saved, err := s.deps.Integrations.UpsertCredential(ctx, sc, cred, secretPlain, extraJSON)
	zeroBytes(secretPlain)
	zeroBytes(extraJSON)
	zeroExtraPlain(extraPlainForZero)
	if err != nil {
		return View{}, err
	}

	if def.Provider == model.IntegrationProviderPM5340 && in.InstallationNumber != nil {
		if err := s.upsertPM5340Analyzer(ctx, sc, def, *in.InstallationNumber); err != nil {
			return View{}, err
		}
	}

	return s.viewOf(ctx, sc, saved, def)
}

// upsertPM5340Analyzer ensures the credential has exactly ONE active
// analyzer, with installationNumber. It is idempotent AND handles a
// CHANGED installation number (I5, fix round 1): the credential's one
// analyzer is identified by (company, provider, subtype) — NOT by
// (provider, subtype, installationNumber). The original draft looked it up
// by the incoming installationNumber via GetByInstallation, which is the
// credential's analyzer's natural key only when the number has never
// changed; an Update that changes InstallationNumber found nothing there
// (the row still has the OLD number) and CREATED A SECOND active pm5340
// analyzer instead of replacing the existing one. A company's pm5340
// credential and its analyzer are 1:1 on (company, provider, subtype) —
// Configure enforces at most one credential per (company, definition), and
// def.Subtype is that definition's — so listing by provider and filtering
// by subtype in Go (AnalyzerFilter carries no subtype field) reliably
// finds THIS credential's one analyzer regardless of what installation
// number it was originally created with.
//
// A changed InstallationNumber retires the old row (SoftDelete) and
// creates a fresh one, rather than updating InstallationNumber in place,
// because AnalyzerRepository.Update never writes
// installation_number/provider/provider_subtype at all — they are that
// repository's own immutable natural key (internal/store/postgres/
// analyzers.go's AnalyzerUpdate query has no such column). This still
// delivers I5's actual requirement — never more than one ACTIVE pm5340
// analyzer for this credential — at the cost of the analyzer getting a new
// id when its installation number changes (its OLD id and any readings
// already attributed to it are retained, just under a soft-deleted row).
//
// This method (rather than the later Discover job) creates/replaces it,
// because pm5340 is "customer-local" (Provider-defaults table:
// "Serialise per company: no (customer-local)") — there is no distributor
// discovery call to find it, only what the operator types in here — so the
// analyzer is created ACTIVE immediately, unlike a provider-discovered one
// (R27, which is about analyzers a sync job finds unprompted).
func (s *Service) upsertPM5340Analyzer(ctx context.Context, sc store.Scope, def model.IntegrationDefinition, installationNumber string) error {
	existing, err := s.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{
		Providers: []model.IntegrationProvider{def.Provider},
	})
	if err != nil {
		return err
	}
	for _, a := range existing {
		if a.ProviderSubtype != def.Subtype {
			continue
		}
		if a.InstallationNumber == installationNumber {
			return nil
		}
		// AnalyzerRepository.Update (internal/store/postgres/analyzers.go)
		// never touches installation_number/provider/provider_subtype —
		// its own natural key — so a changed InstallationNumber cannot be
		// applied in place. Retire the old row (SoftDelete: it drops out
		// of every default List/Get, including the lookup above on the
		// NEXT call) and create a fresh one with the new number, rather
		// than leaving the old one ACTIVE alongside a new one — exactly
		// the "second active pm5340 analyzer" I5 forbids.
		if derr := s.deps.Analyzers.SoftDelete(ctx, sc, a.ID, s.deps.Clock.Now()); derr != nil {
			return derr
		}
		break
	}

	now := s.deps.Clock.Now()
	_, cerr := s.deps.Analyzers.Create(ctx, sc, model.Analyzer{
		CompanyID:          sc.CompanyID,
		Provider:           def.Provider,
		ProviderSubtype:    def.Subtype,
		InstallationNumber: installationNumber,
		MeterMultiplier:    decimal.NewFromInt(1),
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	return cerr
}

// List returns every credential of sc.CompanyID, as Views.
func (s *Service) List(ctx context.Context, sc store.Scope) ([]View, error) {
	if !sc.Valid() {
		return nil, store.ErrInvalidScope
	}
	creds, err := s.deps.Integrations.ListCredentials(ctx, sc)
	if err != nil {
		return nil, err
	}
	defs, err := s.deps.Integrations.Definitions(ctx, sc)
	if err != nil {
		return nil, err
	}
	defByID := make(map[uuid.UUID]model.IntegrationDefinition, len(defs))
	for _, d := range defs {
		defByID[d.ID] = d
	}

	out := make([]View, 0, len(creds))
	for _, c := range creds {
		def, ok := defByID[c.DefinitionID]
		if !ok {
			return nil, store.ErrNotFound
		}
		v, verr := s.viewOf(ctx, sc, c, def)
		if verr != nil {
			return nil, verr
		}
		out = append(out, v)
	}
	return out, nil
}

// Delete removes one credential.
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if !sc.Valid() {
		return store.ErrInvalidScope
	}
	return s.deps.Integrations.DeleteCredential(ctx, sc, id)
}

// Verify proves the credential can authenticate, via the provider's
// Verifier, and stamps last_verified_at (and token_expires_at, for isolar)
// on success. On an authentication failure it returns an error whose text
// has been redacted of every secret fragment this credential carries.
func (s *Service) Verify(ctx context.Context, sc store.Scope, id uuid.UUID) (View, error) {
	if !sc.Valid() {
		return View{}, store.ErrInvalidScope
	}
	creds, err := s.Open(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	verifier, verr := s.deps.Verifiers.Verifier(creds.Provider)
	if verr != nil {
		return View{}, verr
	}
	if verifyErr := verifier.Verify(ctx, creds); verifyErr != nil {
		return View{}, &redactedVerifyError{text: redactedText(creds, verifyErr), err: verifyErr}
	}

	// I1 (fix round 1): re-read the credential now, AFTER Open, instead of
	// trusting creds.TokenExpiresAt. For an isolar credential, Open's call
	// into ISolarAccessToken may have just refreshed the token and already
	// persisted the NEW token_expires_at via persistIsolarToken's own
	// RecordVerification call — but creds (built by buildCredentials
	// BEFORE that refresh happened) still carries the STALE, pre-refresh
	// expiry. Writing creds.TokenExpiresAt here would silently overwrite
	// the correct, just-refreshed expiry with the old one, and the next
	// ISolarAccessToken call would see a soon-to-expire token and refresh
	// AGAIN for no reason. Re-reading picks up whatever is actually
	// current in the store — the refreshed value when Open refreshed,
	// unchanged otherwise (and nil for every non-isolar provider, exactly
	// as before).
	current, _, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return View{}, err
	}

	now := s.deps.Clock.Now()
	if terr := s.deps.Integrations.RecordVerification(ctx, sc, id, now, current.TokenExpiresAt); terr != nil {
		return View{}, terr
	}

	updated, def, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return View{}, err
	}
	return s.viewOf(ctx, sc, updated, def)
}

// Discover enqueues an integration.sync_analyzers task for id, and returns
// the enqueued task's id.
func (s *Service) Discover(ctx context.Context, sc store.Scope, id uuid.UUID) (string, error) {
	if !sc.Valid() {
		return "", store.ErrInvalidScope
	}
	if _, _, err := s.getCredential(ctx, sc, id); err != nil {
		return "", err
	}
	task, err := job.NewSyncAnalyzersTask(
		job.SyncAnalyzersPayload{CompanyID: sc.CompanyID, CredentialID: id},
		job.TaskOptions{MaxRetry: s.deps.MaxRetry},
	)
	if err != nil {
		return "", err
	}
	info, err := s.deps.Enqueuer.Enqueue(ctx, task)
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

// Backfill enqueues an integration.backfill task for id over in's range.
func (s *Service) Backfill(ctx context.Context, sc store.Scope, id uuid.UUID, in BackfillInput) (string, error) {
	if !sc.Valid() {
		return "", store.ErrInvalidScope
	}
	if _, _, err := s.getCredential(ctx, sc, id); err != nil {
		return "", err
	}
	task, err := job.NewBackfillTask(
		job.BackfillPayload{
			CompanyID: sc.CompanyID, CredentialID: id,
			AnalyzerIDs: in.AnalyzerIDs, Kinds: in.Kinds,
			From: in.From, To: in.To, Force: in.Force,
		},
		job.TaskOptions{MaxRetry: s.deps.MaxRetry},
	)
	if err != nil {
		return "", err
	}
	info, err := s.deps.Enqueuer.Enqueue(ctx, task)
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
