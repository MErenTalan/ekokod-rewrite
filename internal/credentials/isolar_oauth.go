package credentials

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ISolarAuthorizeURL builds the iSolarCloud authorisation-request URL (06
// §6 step 1), with an R22 HMAC-signed state naming (sc.CompanyID, id,
// expiry, a fresh nonce) appended to the redirect URI as ?state=....
func (s *Service) ISolarAuthorizeURL(ctx context.Context, sc store.Scope, id uuid.UUID) (string, error) {
	if !sc.Valid() {
		return "", store.ErrInvalidScope
	}
	cred, def, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return "", err
	}
	if def.Provider != model.IntegrationProviderISolar {
		return "", errNotIsolar
	}

	creds, err := s.buildCredentials(ctx, sc, cred, def)
	if err != nil {
		return "", err
	}

	state, err := signState(s.deps.StateKey, sc.CompanyID, id, s.deps.Clock.Now())
	if err != nil {
		return "", err
	}
	redirect := s.deps.RedirectURI + "?state=" + url.QueryEscape(state)

	return s.deps.ISolar.AuthorizeURL(creds, redirect)
}

// ISolarCallback completes 06 §6's authorisation flow (step 3): verify the
// signed state, consume its nonce EXACTLY ONCE (R3), exchange code for a
// Token and store it under the state's own company/credential.
//
// It takes NO Scope: a login produces a Scope, but a provider redirecting
// the browser back to this callback carries none — the signed state IS
// what identifies the company (and the one credential) this callback acts
// for, via store.SystemScope(claims.CompanyID).
//
// Single-use is a non-blocking atomic CONSUME (R3), never a Locker lease
// held and never released: Locker.Acquire POLLS until ctx is done, so a
// replayed callback under that shape would hang for the lease TTL (or
// forever under context.Background()) instead of failing fast. Consume is
// wrapped in its OWN short (2s) timeout, independent of ctx, so that even a
// broken Consumer backend cannot turn a replay into a hang —
// TestStateIsSingleUse asserts the replayed call returns in well under its
// bound, not merely before some longer timeout fires.
func (s *Service) ISolarCallback(ctx context.Context, code, state string) error {
	claims, err := verifyState(s.deps.StateKey, state, s.deps.Clock.Now())
	if err != nil {
		return ErrInvalidState
	}

	cctx, cancel := context.WithTimeout(context.Background(), isolarStateCallbackTimeout)
	defer cancel()
	found, cerr := s.deps.Nonces.Consume(cctx, stateNonceKey(claims.Nonce), stateTTL)
	if cerr != nil || !found {
		// found==false: already consumed (a replay) or the Consumer itself
		// failed — both fail closed, immediately, as ErrInvalidState.
		return ErrInvalidState
	}

	sc := store.SystemScope(claims.CompanyID)
	cred, def, err := s.getCredential(ctx, sc, claims.CredentialID)
	if err != nil {
		return err
	}
	if def.Provider != model.IntegrationProviderISolar {
		return ErrInvalidState
	}

	creds, err := s.buildCredentials(ctx, sc, cred, def)
	if err != nil {
		return err
	}

	tok, err := s.deps.ISolar.ExchangeCode(ctx, creds, code, s.deps.RedirectURI)
	if err != nil {
		return err
	}
	return s.persistIsolarToken(ctx, sc, cred, tok)
}

// ISolarAccessToken returns a valid access token for id, refreshing it
// first when it is expiring within isolarRefreshWindow (06 §6 step 4).
func (s *Service) ISolarAccessToken(ctx context.Context, sc store.Scope, id uuid.UUID) (integration.Secret, error) {
	if !sc.Valid() {
		return integration.Secret{}, store.ErrInvalidScope
	}
	cred, def, err := s.getCredential(ctx, sc, id)
	if err != nil {
		return integration.Secret{}, err
	}
	return s.isolarAccessToken(ctx, sc, id, cred, def)
}

// isolarAccessToken is ISolarAccessToken's body, taking an
// already-resolved cred/def so Open (which has already looked them up) does
// not pay for a second getCredential round trip.
//
// The refresh critical section is DOUBLE-CHECKED locking under
// Deps.Locker (R2, a genuine bounded mutual-exclusion lock, always
// released): the fast path (token fresh) takes no lock at all: a
// concurrent caller that finds the token already fresh (including one that
// arrived after another goroutine's refresh completed) never blocks on the
// lock. Only a caller that observes an expiring token acquires the lock,
// and the VERY FIRST thing it does once it holds it is re-read the
// credential and re-check freshness — because by the time it got the
// lock, another goroutine may have already refreshed it. Skipping that
// re-read (Step 5(b)'s mutation) is exactly what lets N concurrent callers
// each call Refresh once — iSolarCloud invalidates the previous refresh
// token on every refresh (Task 13, 06 §6), so a second call with the
// now-stale refresh_token would fail outright.
func (s *Service) isolarAccessToken(ctx context.Context, sc store.Scope, id uuid.UUID, cred model.IntegrationCredential, def model.IntegrationDefinition) (integration.Secret, error) {
	if def.Provider != model.IntegrationProviderISolar {
		return integration.Secret{}, errNotIsolar
	}

	if access, fresh, err := s.isolarCurrentAccessToken(ctx, sc, cred); err != nil {
		return integration.Secret{}, err
	} else if fresh {
		return access, nil
	}

	lease, err := s.deps.Locker.Acquire(ctx, "isolar:token:"+sc.CompanyID.String(), isolarTokenLockTTL)
	if err != nil {
		return integration.Secret{}, err
	}
	defer func() { _ = lease.Release(ctx) }()

	// Double-checked: re-read now that the lock is held.
	cred, _, err = s.getCredential(ctx, sc, id)
	if err != nil {
		return integration.Secret{}, err
	}
	if access, fresh, err := s.isolarCurrentAccessToken(ctx, sc, cred); err != nil {
		return integration.Secret{}, err
	} else if fresh {
		return access, nil
	}

	creds, err := s.buildCredentials(ctx, sc, cred, def)
	if err != nil {
		return integration.Secret{}, err
	}
	tok, err := s.deps.ISolar.Refresh(ctx, creds)
	if err != nil {
		return integration.Secret{}, err
	}
	if err := s.persistIsolarToken(ctx, sc, cred, tok); err != nil {
		return integration.Secret{}, err
	}
	return tok.AccessToken, nil
}

// isolarCurrentAccessToken reports the currently stored access token and
// whether it is fresh enough (more than isolarRefreshWindow from expiry)
// to hand out without refreshing.
func (s *Service) isolarCurrentAccessToken(ctx context.Context, sc store.Scope, cred model.IntegrationCredential) (integration.Secret, bool, error) {
	if cred.TokenExpiresAt == nil {
		return integration.Secret{}, false, nil
	}
	if !cred.TokenExpiresAt.After(s.deps.Clock.Now().Add(isolarRefreshWindow)) {
		return integration.Secret{}, false, nil
	}

	_, extraPlain, err := s.deps.Integrations.OpenSecret(ctx, sc, cred.ID)
	if err != nil {
		return integration.Secret{}, false, err
	}
	defer zeroBytes(extraPlain)

	var m map[string]string
	if len(extraPlain) > 0 {
		if uerr := json.Unmarshal(extraPlain, &m); uerr != nil {
			return integration.Secret{}, false, uerr
		}
	}
	at, ok := m[isolarExtraAccessToken]
	if !ok || at == "" {
		return integration.Secret{}, false, nil
	}
	return integration.NewSecret([]byte(at)), true, nil
}

// isolarExtraAccessToken/isolarExtraRefreshToken are the Extra keys this
// package's own persisted state uses for isolar's OAuth pair. They mirror
// isolar.Client's own creds.Extra["access_token"]/["refresh_token"] reads
// (internal/integration/isolar/client.go, auth.go) — the two packages must
// agree on these names since this one writes what that one reads.
const (
	isolarExtraAccessToken  = "access_token"
	isolarExtraRefreshToken = "refresh_token"
)

// persistIsolarToken merges tok into cred's stored Extra and stamps
// token_expires_at (RecordVerification). A ZERO tok.RefreshToken
// (IsZero() == true) means the provider issued no new refresh token this
// call (Task 13's isolar.Token doc: "a null refresh_token is legacy's 'no
// new refresh token' case") — the stored one is left exactly as it was,
// NEVER overwritten with a zero value: iSolarCloud invalidates the
// previous refresh token on every successful refresh, so losing track of
// the current one would permanently strand the credential.
func (s *Service) persistIsolarToken(ctx context.Context, sc store.Scope, cred model.IntegrationCredential, tok isolar.Token) error {
	_, extraPlain, err := s.deps.Integrations.OpenSecret(ctx, sc, cred.ID)
	if err != nil {
		return err
	}
	m := map[string]string{}
	if len(extraPlain) > 0 {
		if uerr := json.Unmarshal(extraPlain, &m); uerr != nil {
			zeroBytes(extraPlain)
			return uerr
		}
	}
	zeroBytes(extraPlain)

	m[isolarExtraAccessToken] = tok.AccessToken.Reveal()
	if !tok.RefreshToken.IsZero() {
		m[isolarExtraRefreshToken] = tok.RefreshToken.Reveal()
	}

	extraJSON, merr := json.Marshal(m)
	if merr != nil {
		zeroExtraPlain(m)
		return merr
	}

	upsert := model.IntegrationCredential{
		ID: cred.ID, CompanyID: cred.CompanyID, DefinitionID: cred.DefinitionID,
		Username: cred.Username, Settings: cred.Settings,
		Pm5340URL: cred.Pm5340URL, IsolarRegion: cred.IsolarRegion, IsActive: cred.IsActive,
	}
	_, err = s.deps.Integrations.UpsertCredential(ctx, sc, upsert, nil, extraJSON)
	zeroBytes(extraJSON)
	zeroExtraPlain(m)
	if err != nil {
		return err
	}

	expiresAt := tok.ExpiresAt
	return s.deps.Integrations.RecordVerification(ctx, sc, cred.ID, s.deps.Clock.Now(), &expiresAt)
}
