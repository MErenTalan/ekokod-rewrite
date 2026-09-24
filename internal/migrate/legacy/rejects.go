package legacy

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"sync"
)

// Tally is one collection's outcome.
type Tally struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

// Rejections records every refused record with its reason and keeps the
// input = accepted + rejected invariant checkable (R404).
type Rejections struct {
	mu     sync.Mutex
	out    *json.Encoder
	tally  map[string]*Tally
	failed error
}

// NewRejections writes NDJSON rejection lines to w.
func NewRejections(w io.Writer) *Rejections {
	return &Rejections{out: json.NewEncoder(w), tally: map[string]*Tally{}}
}

func (r *Rejections) of(collection string) *Tally {
	t, ok := r.tally[collection]
	if !ok {
		t = &Tally{}
		r.tally[collection] = t
	}
	return t
}

// Accept counts one record that made it through.
func (r *Rejections) Accept(collection string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.of(collection).Accepted++
}

// Reject records one refused record; nothing is dropped silently.
func (r *Rejections) Reject(collection, legacyID, field, reason string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.of(collection).Rejected++
	if err := r.out.Encode(map[string]any{"collection": collection, "legacy_id": legacyID, "field": field, "reason": reason, "value": value}); err != nil && r.failed == nil {
		r.failed = err
	}
}

// Check proves every one of `read` records was accepted or rejected.
func (r *Rejections) Check(collection string, read int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failed != nil {
		return fmt.Errorf("legacy: writing rejections: %w", r.failed)
	}
	t := r.of(collection)
	if t.Accepted+t.Rejected != read {
		return fmt.Errorf("legacy: %s: %d read, %d accepted + %d rejected — records went missing", collection, read, t.Accepted, t.Rejected)
	}
	return nil
}

// Summary is every collection's tally, for the run report.
func (r *Rejections) Summary() map[string]Tally {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.tally))
	for n := range r.tally {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make(map[string]Tally, len(names))
	for _, n := range names {
		out[n] = *r.tally[n]
	}
	return out
}
