// Package demo answers whether a company is the shared demo company (R540),
// so outbound effects can be suppressed for it without comparing ids.
package demo

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Check reports whether companyID is a demo company.
type Check func(ctx context.Context, companyID uuid.UUID) (bool, error)

// CompanyGetter is the slice of store.CompanyRepository FromCompanies needs.
type CompanyGetter interface {
	Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Company, error)
}

// FromCompanies loads the flag from the companies table; a missing company is not a demo.
func FromCompanies(repo CompanyGetter) Check {
	return func(ctx context.Context, id uuid.UUID) (bool, error) {
		c, err := repo.Get(ctx, store.SystemScope(id), id)
		if errors.Is(err, store.ErrNotFound) {
			return false, nil
		}
		return c.IsDemo, err
	}
}

// Checker caches Check results per company for ttl; errors are never cached.
type Checker struct {
	load Check
	ttl  time.Duration
	now  func() time.Time

	mu      sync.Mutex
	entries map[uuid.UUID]entry
}

type entry struct {
	demo bool
	at   time.Time
}

// NewChecker builds a Checker over load.
func NewChecker(load Check, ttl time.Duration, now func() time.Time) *Checker {
	return &Checker{load: load, ttl: ttl, now: now, entries: map[uuid.UUID]entry{}}
}

// IsDemo implements Check.
func (c *Checker) IsDemo(ctx context.Context, companyID uuid.UUID) (bool, error) {
	now := c.now()
	c.mu.Lock()
	e, ok := c.entries[companyID]
	c.mu.Unlock()
	if ok && now.Sub(e.at) <= c.ttl {
		return e.demo, nil
	}
	got, err := c.load(ctx, companyID)
	if err != nil {
		return false, err
	}
	c.mu.Lock()
	c.entries[companyID] = entry{demo: got, at: now}
	c.mu.Unlock()
	return got, nil
}
