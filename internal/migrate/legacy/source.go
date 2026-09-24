package legacy

import (
	"context"
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Source is the legacy database, read-only by construction: nothing here can write (R406).
type Source interface {
	// Collections lists collection names.
	Collections(ctx context.Context) ([]string, error)
	// Iterate walks a collection in _id order.
	Iterate(ctx context.Context, collection string, fn func(bson.Raw) error) error
}

// MemSource is an in-memory Source for tests and fixtures.
type MemSource struct{ docs map[string][]bson.Raw }

// NewMemSource builds an empty source.
func NewMemSource() *MemSource { return &MemSource{docs: map[string][]bson.Raw{}} }

// Add appends documents to a collection.
func (m *MemSource) Add(collection string, docs ...bson.D) {
	for _, d := range docs {
		raw, err := bson.Marshal(d)
		if err != nil {
			panic(fmt.Sprintf("memsource: %v", err))
		}
		m.docs[collection] = append(m.docs[collection], raw)
	}
}

// Replace swaps a collection's documents.
func (m *MemSource) Replace(collection string, docs ...bson.D) {
	delete(m.docs, collection)
	m.Add(collection, docs...)
}

// Collections lists the collections that have documents.
func (m *MemSource) Collections(context.Context) ([]string, error) {
	out := make([]string, 0, len(m.docs))
	for c := range m.docs {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// Iterate walks a collection in _id order.
func (m *MemSource) Iterate(_ context.Context, collection string, fn func(bson.Raw) error) error {
	docs := append([]bson.Raw(nil), m.docs[collection]...)
	sort.SliceStable(docs, func(i, j int) bool { return idKey(docs[i]) < idKey(docs[j]) })
	for _, d := range docs {
		if err := fn(d); err != nil {
			return err
		}
	}
	return nil
}

func idKey(doc bson.Raw) string {
	v := doc.Lookup("_id")
	if id, ok := v.ObjectIDOK(); ok {
		return id.Hex()
	}
	return v.String()
}
