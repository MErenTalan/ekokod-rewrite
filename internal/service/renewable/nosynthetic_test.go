package renewable_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
)

// flatten turns a panel into panel/field → JSON value, descending into nested
// objects (analytics.financial) and skipping the unavailable map and series.
func flatten(t *testing.T, prefix string, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	out := map[string]json.RawMessage{}
	for k, val := range m {
		if k == "unavailable" {
			continue
		}
		if strings.HasPrefix(string(val), "{") && !strings.Contains(k, "unavailable") {
			for kk, vv := range flatten(t, "", json.RawMessage(val)) {
				out[prefix+k+"."+kk] = vv
			}
			continue
		}
		out[prefix+k] = val
	}
	return out
}

func unavailable(t *testing.T, v any) map[string]string {
	t.Helper()
	raw, _ := json.Marshal(v)
	var m struct {
		Unavailable map[string]string `json:"unavailable"`
		Financial   struct {
			Unavailable map[string]string `json:"unavailable"`
		} `json:"financial"`
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	out := map[string]string{}
	for k, v := range m.Unavailable {
		out[k] = v
	}
	for k, v := range m.Financial.Unavailable {
		out["financial."+k] = v
	}
	return out
}

func panels(in renewable.Inputs) map[string]any {
	return map[string]any{
		"overview": renewable.BuildOverview(in), "realtime": renewable.BuildRealtime(in),
		"grid_interaction": renewable.BuildGridInteraction(in), "environmental": renewable.BuildEnvironmental(in),
		"efficiency": renewable.BuildEfficiency(in), "forecast": renewable.BuildForecast(in),
		"analytics": renewable.BuildAnalytics(in), "system_status": renewable.BuildSystemStatus(in),
	}
}

func isNumber(raw json.RawMessage) bool {
	s := strings.Trim(string(raw), `"`)
	_, err := decimal.NewFromString(s)
	return err == nil && strings.HasPrefix(string(raw), `"`)
}

// TestNoSyntheticData is 09 §F9's acceptance test (R292, 10 item 12): every
// field of every renewable panel has a declared source; measured and derived
// figures move with their inputs (no constants), repeat exactly (no random),
// are null without data (missing is not zero); unavailable fields are null
// and say why.
func TestNoSyntheticData(t *testing.T) {
	a, b, empty := panels(fixture("1", "0", 36)), panels(fixture("1.7", "0.3", 30)), panels(renewable.Inputs{Now: now, From: now.AddDate(0, 0, -7), To: now})
	again := panels(fixture("1", "0", 36))

	for name, panel := range a {
		sources, ok := renewable.Sources[name]
		require.True(t, ok, "panel %s has no Sources entry", name)
		fields := flatten(t, "", panel)
		var got, declared []string
		for f := range fields {
			got = append(got, f)
		}
		for f := range sources {
			declared = append(declared, f)
		}
		sort.Strings(got)
		sort.Strings(declared)
		require.Equal(t, declared, got, "panel %s: every JSON field must be declared in Sources and vice versa", name)

		require.Equal(t, fields, flatten(t, "", again[name]), "panel %s is not deterministic", name)
		other := flatten(t, "", b[name])
		none := flatten(t, "", empty[name])
		why := unavailable(t, panel)
		for field, src := range sources {
			switch src.Kind {
			case renewable.Unavailable:
				require.Equal(t, "null", string(fields[field]), "%s.%s is declared unavailable but has a value", name, field)
				require.NotEmpty(t, why[field], "%s.%s is unavailable without a reason", name, field)
			case renewable.Measured, renewable.Derived:
				require.NotEmpty(t, src.From, "%s.%s names no source", name, field)
				if isNumber(fields[field]) {
					require.NotEqual(t, string(fields[field]), string(other[field]),
						"%s.%s did not change when its inputs changed: a constant?", name, field)
					require.Equal(t, "null", string(none[field]), "%s.%s has a value without any data: missing must not be zero", name, field)
				}
			case renewable.Label:
				require.NotEmpty(t, src.From, "%s.%s names no source", name, field)
			default:
				t.Fatalf("%s.%s has an unknown source kind %q", name, field, src.Kind)
			}
		}
	}
	require.Len(t, renewable.Sources, len(a), "Sources declares a panel no builder returns")
	_ = reflect.TypeOf
}
