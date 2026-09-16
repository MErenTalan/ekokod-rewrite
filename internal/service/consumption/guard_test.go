package consumption_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// I-8: the original R61 guard tests only proved a cross-wired fallback
// dereferenced a nil field — they never inspected the TYPE of the field
// itself, so a defensive extra field (say, `Readings store.ReadingRepository`
// on AnalyticsDeps) plus a nil-guarded fallback call left the suite GREEN as
// long as the fallback's own code path was never exercised. This file
// replaces that guard with a reflection walk over the struct's field TYPES:
// no runtime call is needed to prove the field should not exist at all.

// TestAnalyticsDepsHasNoReadingOrAnomalyRepositoryField is I-8's reflection
// guard for the Analytics side: no field of AnalyticsDeps may have a type
// that equals, implements, or (through a struct/pointer/slice/array/map)
// contains store.ReadingRepository or store.AnomalyRepository.
func TestAnalyticsDepsHasNoReadingOrAnomalyRepositoryField(t *testing.T) {
	readingRepo := reflect.TypeOf((*store.ReadingRepository)(nil)).Elem()
	anomalyRepo := reflect.TypeOf((*store.AnomalyRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.AnalyticsDeps{}), readingRepo, anomalyRepo)
}

// TestBillingDepsHasNoAnalyticsRepositoryField is I-8's reflection guard for
// the Billing side: no field of BillingDeps may have a type that equals,
// implements, or contains store.AnalyticsRepository.
func TestBillingDepsHasNoAnalyticsRepositoryField(t *testing.T) {
	analyticsRepo := reflect.TypeOf((*store.AnalyticsRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.BillingDeps{}), analyticsRepo)
}

// TestAnalyticsStructItselfHasNoForbiddenField and
// TestBillingStructItselfHasNoForbiddenField are Minor M-a: the original
// guard walked only the *Deps structs, so an unexported field of a forbidden
// type added directly on Analytics or Billing (bypassing *Deps entirely)
// stayed green. The reflection walk is the same; only the root type differs
// (unexported fields are still visible to reflect.Type.Field, unlike
// reflect.Value — no instance is ever created, so there is nothing to fail
// to read).
func TestAnalyticsStructItselfHasNoForbiddenField(t *testing.T) {
	readingRepo := reflect.TypeOf((*store.ReadingRepository)(nil)).Elem()
	anomalyRepo := reflect.TypeOf((*store.AnomalyRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.Analytics{}), readingRepo, anomalyRepo)
}

func TestBillingStructItselfHasNoForbiddenField(t *testing.T) {
	analyticsRepo := reflect.TypeOf((*store.AnalyticsRepository)(nil)).Elem()
	assertNoForbiddenField(t, reflect.TypeOf(consumption.Billing{}), analyticsRepo)
}

// TestFindForbiddenFieldCatchesAFuncTypedField is Minor M-a: a field typed
// as a function that RETURNS the forbidden interface (a lazy accessor a
// defensive fallback could call instead of storing the repository directly)
// must be caught. The original walk had no reflect.Func case at all, so
// this stayed invisible. findForbiddenField (below) is the pure,
// non-t.Fatalf-calling half of the guard, so this test can assert the
// mutation IS detected without ever failing this test itself.
func TestFindForbiddenFieldCatchesAFuncTypedField(t *testing.T) {
	readingRepo := reflect.TypeOf((*store.ReadingRepository)(nil)).Elem()
	type poisoned struct {
		Get func() store.ReadingRepository
	}
	path, found := findForbiddenField(reflect.TypeOf(poisoned{}), readingRepo)
	require.True(t, found, "a field typed func() store.ReadingRepository must be caught, not silently skipped")
	require.Contains(t, path, "Get")
}

// TestFindForbiddenFieldCatchesAMapKeyTypedField is Minor M-e (re-review
// round 2): the original Map case walked only Elem(), never Key(), so a
// field typed `map[store.ReadingRepository]bool` (the forbidden repository
// used as a map KEY rather than a value) stayed invisible to this guard.
// findForbiddenField now walks both Key() and Elem() for reflect.Map, so
// this probe — mirroring TestFindForbiddenFieldCatchesAFuncTypedField's
// shape for the Func case — must be caught.
func TestFindForbiddenFieldCatchesAMapKeyTypedField(t *testing.T) {
	readingRepo := reflect.TypeOf((*store.ReadingRepository)(nil)).Elem()
	type poisoned struct {
		ByRepo map[store.ReadingRepository]bool
	}
	path, found := findForbiddenField(reflect.TypeOf(poisoned{}), readingRepo)
	require.True(t, found, "a field typed map[store.ReadingRepository]bool must be caught via its KEY type, not silently skipped")
	require.Contains(t, path, "ByRepo")
}

// assertNoForbiddenField fails the test immediately if findForbiddenField
// finds a match — the reporting half of the guard every real R61 guard test
// above calls.
func assertNoForbiddenField(t *testing.T, typ reflect.Type, forbidden ...reflect.Type) {
	t.Helper()
	if path, found := findForbiddenField(typ, forbidden...); found {
		t.Fatalf("%s — R61 forbids this on this side of the path split", path)
	}
}

// findForbiddenField walks typ's fields recursively — into structs,
// pointers, slices, arrays, maps and (Minor M-a) function signatures — and
// returns the first field path whose own type equals or implements one of
// forbidden, and whether one was found at all. seen guards against
// revisiting the same type twice (both for efficiency and to tolerate any
// accidental type cycle).
//
// M-a: the original walk skipped reflect.Func entirely, so a field typed
// `func() store.ReadingRepository` (a lazy accessor a defensive fallback
// could call instead of storing the repository directly) stayed invisible
// to this guard. Every parameter and every return type of a Func field is
// now walked exactly like any other field's type.
//
// M-e: the original Map case walked only Elem(), never Key(), so a field
// typed `map[store.ReadingRepository]T` (the forbidden repository used as
// the map's KEY rather than its value) stayed invisible. Both Key() and
// Elem() are now walked for reflect.Map.
func findForbiddenField(typ reflect.Type, forbidden ...reflect.Type) (string, bool) {
	seen := make(map[reflect.Type]bool)
	var path string
	var found bool
	var walk func(cur reflect.Type, p string)
	walk = func(cur reflect.Type, p string) {
		if found || cur == nil || seen[cur] {
			return
		}
		seen[cur] = true

		for _, iface := range forbidden {
			if cur == iface || cur.Implements(iface) {
				path = fmt.Sprintf("%s has type %s, which is or implements %s", p, cur, iface)
				found = true
				return
			}
		}

		switch cur.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(cur.Elem(), p+"[]")
		case reflect.Map:
			walk(cur.Key(), p+"[key]")
			walk(cur.Elem(), p+"[]")
		case reflect.Struct:
			for i := 0; i < cur.NumField(); i++ {
				f := cur.Field(i)
				walk(f.Type, p+"."+f.Name)
			}
		case reflect.Func:
			for i := 0; i < cur.NumIn(); i++ {
				walk(cur.In(i), fmt.Sprintf("%s(in %d)", p, i))
			}
			for i := 0; i < cur.NumOut(); i++ {
				walk(cur.Out(i), fmt.Sprintf("%s(out %d)", p, i))
			}
		}
	}
	walk(typ, typ.Name())
	return path, found
}
