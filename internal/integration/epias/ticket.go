package epias

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
)

// ticketLockKey is the fixed cross-process lock name Options.Locker
// serialises TGT acquisition under (task-12 brief: 'holds Locker
// "epias:tgt"').
const ticketLockKey = "epias:tgt"

// ticketLockTTL matches the common "lock TTL is 60s" default (Provider
// defaults table, "Common to all").
const ticketLockTTL = 60 * time.Second

// ticketPattern matches a CAS TGT identifier, e.g. "TGT-12-ab3F...-cas01".
var ticketPattern = regexp.MustCompile(`TGT-[A-Za-z0-9._-]+`)

// authRequest is the ONE helper every CAS ticket-granting call goes
// through (R32): it always sets NoRetry, so a second, differently built
// auth POST can never accidentally skip it. The request body is a
// standard application/x-www-form-urlencoded "username=…&password=…" form
// — the Apereo CAS REST protocol EPİAŞ's CAS endpoint implements — never a
// query string, so neither credential is ever part of a URL httpx might
// (in error text elsewhere) need to handle specially.
func authRequest(casURL, username, password string) httpx.Request {
	form := url.Values{"username": {username}, "password": {password}}
	return httpx.Request{
		Op:          "auth",
		Method:      http.MethodPost,
		Template:    casURL,
		Body:        []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
		// R32: retrying a login POST asks CAS to mint a second ticket
		// after a transient failure on the first attempt — on the real
		// CAS protocol a fresh grant does not necessarily invalidate a
		// concurrently-held one, but there is no benefit to a second
		// attempt either (getTicket already holds a lock and will simply
		// call this again on genuine failure), so exactly one attempt,
		// always, is both correct and the documented ruling.
		NoRetry: true,
	}
}

// extractTicket finds the "TGT-…" ticket in resp's Location header,
// falling back to its body (06 §7 / legacy epiasService.ts:124-139).
func extractTicket(resp httpx.Response) (string, bool) {
	if loc := resp.Header.Get("Location"); loc != "" {
		if t := ticketPattern.FindString(loc); t != "" {
			return t, true
		}
	}
	if t := ticketPattern.FindString(string(resp.Body)); t != "" {
		return t, true
	}
	return "", false
}

// fetchTicket performs one CAS round trip and returns the ticket. A
// non-201 response, or a 201 with no extractable ticket, is ErrAuth
// (task-12 brief: "A 401, or a 201 with no TGT, is ErrAuth" — httpx's own
// classify.go already turns a 401 into ErrAuth before this method ever
// sees it, since Do only ever returns a *integration.Error for a
// non-2xx status; the explicit status check below therefore only ever
// catches an unexpected-but-2xx status, e.g. 200 where 201 was
// contracted).
func (c *Client) fetchTicket(ctx context.Context) (string, error) {
	resp, err := c.casClient.Do(ctx, authRequest(c.opts.CASURL, c.opts.Username, c.opts.Password.Reveal()))
	if err != nil {
		return "", err
	}
	if resp.Status != http.StatusCreated {
		return "", &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderEPIAS, Op: "auth", HTTPStatus: resp.Status}
	}
	ticket, ok := extractTicket(resp)
	if !ok {
		return "", &integration.Error{Kind: integration.ErrAuth, Provider: integration.ProviderEPIAS, Op: "auth", HTTPStatus: resp.Status}
	}
	return ticket, nil
}

// cachedTicket reports the ticket currently cached, and whether it is
// still within TicketTTL of when it was obtained.
func (c *Client) cachedTicket() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ticket == "" {
		return "", false
	}
	if !c.opts.Clock.Now().Before(c.ticketAt.Add(c.opts.TicketTTL)) {
		return "", false
	}
	return c.ticket, true
}

func (c *Client) storeTicket(ticket string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ticket = ticket
	c.ticketAt = c.opts.Clock.Now()
}

func (c *Client) invalidateTicket() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ticket = ""
}

// getTicket returns a valid TGT, from cache when possible. Obtaining a
// fresh one holds Options.Locker(ticketLockKey) — when one is configured —
// and re-checks the cache after acquiring it (double-checked locking):
// two goroutines racing to refresh an expired ticket must not both call
// CAS. The second to acquire the lock finds the first one's fresh ticket
// already cached by the time it gets in, and returns that instead of
// making a second CAS round trip. Options.Locker == nil (unit tests only)
// skips the cross-process lock entirely; getTicket's own mutex-guarded
// cache read/write still makes concurrent callers within one process safe,
// just not free of a duplicate CAS call if they race before either has
// cached anything.
func (c *Client) getTicket(ctx context.Context) (string, error) {
	if t, ok := c.cachedTicket(); ok {
		return t, nil
	}

	if c.opts.Locker != nil {
		lease, err := c.opts.Locker.Acquire(ctx, ticketLockKey, ticketLockTTL)
		if err != nil {
			return "", &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderEPIAS, Op: "auth"}
		}
		defer func() {
			releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = lease.Release(releaseCtx)
			cancel()
		}()

		if t, ok := c.cachedTicket(); ok {
			return t, nil
		}
	}

	ticket, err := c.fetchTicket(ctx)
	if err != nil {
		return "", err
	}
	c.storeTicket(ticket)
	return ticket, nil
}
