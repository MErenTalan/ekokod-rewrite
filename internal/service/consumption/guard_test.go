package consumption_test

import (
	"reflect"
	"testing"

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

// assertNoForbiddenField walks typ's fields recursively — into structs,
// pointers, slices, arrays and maps — and fails the test if any field's own
// type equals or implements one of forbidden. seen guards against revisiting
// the same type twice (both for efficiency and to tolerate any accidental
// type cycle).
func assertNoForbiddenField(t *testing.T, typ reflect.Type, forbidden ...reflect.Type) {
	t.Helper()
	seen := make(map[reflect.Type]bool)
	var walk func(cur reflect.Type, path string)
	walk = func(cur reflect.Type, path string) {
		if cur == nil || seen[cur] {
			return
		}
		seen[cur] = true

		for _, iface := range forbidden {
			if cur == iface || cur.Implements(iface) {
				t.Fatalf("%s has type %s, which is or implements %s — R61 forbids this on this side of the path split", path, cur, iface)
			}
		}

		switch cur.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(cur.Elem(), path+"[]")
		case reflect.Map:
			walk(cur.Elem(), path+"[]")
		case reflect.Struct:
			for i := 0; i < cur.NumField(); i++ {
				f := cur.Field(i)
				walk(f.Type, path+"."+f.Name)
			}
		}
	}
	walk(typ, typ.Name())
}
