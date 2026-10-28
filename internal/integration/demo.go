package integration

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// GuardDemo wraps a meter adapter so a demo company's credentials never reach
// the provider (R545): verification succeeds, discovery finds nothing new and
// a fetch returns no readings. The demo's readings come from the seed instead.
func GuardDemo(a Adapter, isDemo func(context.Context, uuid.UUID) (bool, error)) Adapter {
	return demoAdapter{inner: a, isDemo: isDemo}
}

type demoAdapter struct {
	inner  Adapter
	isDemo func(context.Context, uuid.UUID) (bool, error)
}

func (d demoAdapter) Provider() Provider                          { return d.inner.Provider() }
func (d demoAdapter) Kinds(creds Credentials) []model.ReadingKind { return d.inner.Kinds(creds) }
func (d demoAdapter) MaxWindow(k model.ReadingKind) time.Duration { return d.inner.MaxWindow(k) }

func (d demoAdapter) Verify(ctx context.Context, creds Credentials) error {
	if demo, err := d.isDemo(ctx, creds.CompanyID); err != nil || demo {
		return err
	}
	return d.inner.Verify(ctx, creds)
}

func (d demoAdapter) DiscoverMeteringPoints(ctx context.Context, creds Credentials) ([]MeteringPoint, error) {
	if demo, err := d.isDemo(ctx, creds.CompanyID); err != nil || demo {
		return nil, err
	}
	return d.inner.DiscoverMeteringPoints(ctx, creds)
}

func (d demoAdapter) FetchReadings(ctx context.Context, creds Credentials, req FetchRequest) (FetchResult, error) {
	if demo, err := d.isDemo(ctx, creds.CompanyID); err != nil || demo {
		return FetchResult{}, err
	}
	return d.inner.FetchReadings(ctx, creds, req)
}

// IsGuarded reports whether a was built by GuardDemo (the wiring guard test).
func IsGuarded(a Adapter) bool {
	_, ok := a.(demoAdapter)
	return ok
}
