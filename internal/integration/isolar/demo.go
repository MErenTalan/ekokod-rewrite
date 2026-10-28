package isolar

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

// DemoPSID is the seeded Konya plant's iSolar id; DemoPlants is what a demo
// company's plant picker lists.
const DemoPSID = "DEMO-PS-1001"

// DemoPlants are the plants a demo company's iSolar account "has".
var DemoPlants = []Plant{{PSID: DemoPSID, Name: "Konya Arazi GES", InstalledKw: ptr(decimal.NewFromInt(1000))}}

func ptr[T any](v T) *T { return &v }

// Upstream is every *Client method the services use.
type Upstream interface {
	Plants(ctx context.Context, creds integration.Credentials) ([]Plant, error)
	Devices(ctx context.Context, creds integration.Credentials, psID string) ([]Device, error)
	DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]YieldSample, error)
	PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]YieldSample, error)
	PlantDailySeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]DailyYield, error)
	DeviceRealtime(ctx context.Context, creds integration.Credentials, deviceType int32, psKeys []string) ([]DeviceSnapshot, error)
	Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]Fault, error)
	AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error)
	ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (Token, error)
	Refresh(ctx context.Context, creds integration.Credentials) (Token, error)
	Verify(ctx context.Context, creds integration.Credentials) error
}

var _ Upstream = (*Client)(nil)

// Guarded is an Upstream that never calls iSolarCloud for a demo company (R545).
type Guarded struct {
	inner  Upstream
	isDemo func(context.Context, uuid.UUID) (bool, error)
	now    func() time.Time
}

// GuardDemo wraps c; now dates the fake demo tokens.
func GuardDemo(c Upstream, isDemo func(context.Context, uuid.UUID) (bool, error), now func() time.Time) *Guarded {
	return &Guarded{inner: c, isDemo: isDemo, now: now}
}

// demo is ok=true when the call must not leave the server.
func (g *Guarded) demo(ctx context.Context, creds integration.Credentials) (bool, error) {
	return g.isDemo(ctx, creds.CompanyID)
}

func (g *Guarded) token() Token {
	return Token{AccessToken: integration.NewSecret([]byte("demo-access")), RefreshToken: integration.NewSecret([]byte("demo-refresh")),
		ExpiresAt: g.now().Add(24 * time.Hour)}
}

func (g *Guarded) Plants(ctx context.Context, creds integration.Credentials) ([]Plant, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return DemoPlants, err
	}
	return g.inner.Plants(ctx, creds)
}

func (g *Guarded) Devices(ctx context.Context, creds integration.Credentials, psID string) ([]Device, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.Devices(ctx, creds, psID)
}

func (g *Guarded) DeviceMinuteSeries(ctx context.Context, creds integration.Credentials, psKeys []string, from, to time.Time) ([]YieldSample, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.DeviceMinuteSeries(ctx, creds, psKeys, from, to)
}

func (g *Guarded) PlantMinuteSeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]YieldSample, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.PlantMinuteSeries(ctx, creds, psID, from, to)
}

func (g *Guarded) PlantDailySeries(ctx context.Context, creds integration.Credentials, psID string, from, to time.Time) ([]DailyYield, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.PlantDailySeries(ctx, creds, psID, from, to)
}

func (g *Guarded) DeviceRealtime(ctx context.Context, creds integration.Credentials, deviceType int32, psKeys []string) ([]DeviceSnapshot, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.DeviceRealtime(ctx, creds, deviceType, psKeys)
}

func (g *Guarded) Faults(ctx context.Context, creds integration.Credentials, from, to time.Time) ([]Fault, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return nil, err
	}
	return g.inner.Faults(ctx, creds, from, to)
}

// AuthorizeURL sends a demo company straight back to our own callback with a
// fake code, so the normal cookie-checked callback links it (R545).
func (g *Guarded) AuthorizeURL(creds integration.Credentials, redirectURI string) (string, error) {
	if demo, err := g.demo(context.Background(), creds); err != nil || demo {
		sep := "?"
		if strings.Contains(redirectURI, "?") {
			sep = "&"
		}
		return redirectURI + sep + "code=demo", err
	}
	return g.inner.AuthorizeURL(creds, redirectURI)
}

func (g *Guarded) ExchangeCode(ctx context.Context, creds integration.Credentials, code, redirectURI string) (Token, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return g.token(), err
	}
	return g.inner.ExchangeCode(ctx, creds, code, redirectURI)
}

func (g *Guarded) Refresh(ctx context.Context, creds integration.Credentials) (Token, error) {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return g.token(), err
	}
	return g.inner.Refresh(ctx, creds)
}

func (g *Guarded) Verify(ctx context.Context, creds integration.Credentials) error {
	if demo, err := g.demo(ctx, creds); err != nil || demo {
		return err
	}
	return g.inner.Verify(ctx, creds)
}
