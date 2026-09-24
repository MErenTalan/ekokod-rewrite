package legacy

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var readOnlyCommands = map[string]bool{
	"find": true, "getMore": true, "count": true, "aggregate": true, "listCollections": true, "listIndexes": true,
	"hello": true, "isMaster": true, "ismaster": true, "ping": true, "buildInfo": true, "saslStart": true, "saslContinue": true,
	"killCursors": true, "endSessions": true, "getParameter": true,
}

// ReadOnlyCommand reports whether a wire command cannot write (R406).
func ReadOnlyCommand(name string) bool { return readOnlyCommands[name] }

// Guard records any write command the driver issues; every read checks it.
type Guard struct {
	mu        sync.Mutex
	violation []string
}

// NewGuard builds an empty guard.
func NewGuard() *Guard { return &Guard{} }

// Observe records one command.
func (g *Guard) Observe(command string) {
	if ReadOnlyCommand(command) {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.violation = append(g.violation, command)
}

// Err is non-nil once any write was attempted.
func (g *Guard) Err() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.violation) > 0 {
		return fmt.Errorf("legacy: refusing to continue — the migration issued write commands %v against the legacy database", g.violation)
	}
	return nil
}

// MongoSource reads the legacy database with find/listCollections only (R406).
type MongoSource struct {
	client *mongo.Client
	db     *mongo.Database
	guard  *Guard
}

// OpenMongo connects with a command monitor that watches for writes; the
// connection should also use a read-only database user.
func OpenMongo(ctx context.Context, uri, database string) (*MongoSource, error) {
	guard := NewGuard()
	monitor := &event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) { guard.Observe(e.CommandName) }}
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetMonitor(monitor).SetAppName("ekokod-migrate-legacy"))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	return &MongoSource{client: client, db: client.Database(database), guard: guard}, nil
}

// Close disconnects.
func (m *MongoSource) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }

// Collections lists collection names, sorted.
func (m *MongoSource) Collections(ctx context.Context) ([]string, error) {
	names, err := m.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, m.guard.Err()
}

// Iterate walks a collection in _id order.
func (m *MongoSource) Iterate(ctx context.Context, collection string, fn func(bson.Raw) error) error {
	cur, err := m.db.Collection(collection).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return err
	}
	defer func() { _ = cur.Close(ctx) }()
	for cur.Next(ctx) {
		if err := m.guard.Err(); err != nil {
			return err
		}
		if err := fn(cur.Current); err != nil {
			return err
		}
	}
	if err := cur.Err(); err != nil {
		return err
	}
	return m.guard.Err()
}
