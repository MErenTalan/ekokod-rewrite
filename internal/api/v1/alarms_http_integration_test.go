//go:build integration

package v1_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// commsBody is a data-communication rule on one analyzer, the cheapest rule to
// create: no readings are needed for it to evaluate (R216).
func commsBody(h *harness, hours int, analyzers ...string) map[string]any {
	if len(analyzers) == 0 {
		analyzers = []string{h.fx.AnalyzerA1.String()}
	}
	return map[string]any{
		"name": "İletişim", "type": "data_communication", "is_enabled": true,
		"analyzer_ids": analyzers,
		"channels":     []map[string]string{{"channel": "email", "target": "ops@example.com"}},
		"settings":     map[string]any{"communication_threshold_hours": hours},
	}
}

func TestAlarmCRUDRoundTrip(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.do(http.MethodPost, "/alarms", commsBody(h, 6))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.Alarm
	res.json(t, &created)
	require.Len(t, created.Analyzers, 1)
	require.Equal(t, h.fx.AnalyzerA1, created.Analyzers[0].ID)
	require.NotEmpty(t, created.Analyzers[0].InstallationNumber)
	require.Len(t, created.Channels, 1)
	require.Equal(t, "ops@example.com", created.Channels[0].Target)
	require.EqualValues(t, 6, *created.Settings.CommunicationThresholdHours)

	// A building admin of A1 sees it: R213's intersection, not the company row.
	listed := h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/alarms", nil)
	require.Equal(t, http.StatusOK, listed.status, string(listed.body))
	var page dto.Page[dto.Alarm]
	listed.json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, created.ID, page.Items[0].ID)

	// PATCH is a full replace: the threshold changes and nothing lingers.
	updated := commsBody(h, 12)
	updated["name"] = "İletişim v2"
	res = ca.do(http.MethodPatch, "/alarms/"+created.ID.String(), updated)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var after dto.Alarm
	res.json(t, &after)
	require.Equal(t, "İletişim v2", after.Name)
	require.EqualValues(t, 12, *after.Settings.CommunicationThresholdHours)

	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/alarms/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/alarms/"+created.ID.String(), nil).status)
}

func TestAlarmVoltageIsRejectedWith422(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.do(http.MethodPost, "/alarms", map[string]any{
		"name": "Gerilim", "type": "current_voltage_power", "is_enabled": true,
		"analyzer_ids": []string{h.fx.AnalyzerA1.String()},
		"settings":     map[string]any{"power_max": "100", "voltage_max": "400"},
	})
	// R212: named, not silently dropped. A silent drop is the failure mode the
	// ruling exists to prevent.
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	require.Equal(t, "validation_failed", res.code(t))
	require.Contains(t, string(res.body), "voltage_max")

	// Without it the same rule is accepted.
	res = ca.do(http.MethodPost, "/alarms", map[string]any{
		"name": "Güç", "type": "current_voltage_power", "is_enabled": true,
		"analyzer_ids": []string{h.fx.AnalyzerA1.String()},
		"settings":     map[string]any{"power_max": "100"},
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))

	// And the response never renders a voltage field back.
	var created dto.Alarm
	res.json(t, &created)
	require.Nil(t, created.Settings.VoltageMax)
	require.Nil(t, created.Settings.VoltageMin)
}

func TestAlarmStoredVoltageIsNeverRenderedBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/alarms", map[string]any{
		"name": "Güç", "type": "current_voltage_power", "is_enabled": true,
		"analyzer_ids": []string{h.fx.AnalyzerA1.String()},
		"settings":     map[string]any{"power_max": "100"},
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var rule dto.Alarm
	res.json(t, &rule)

	// The API cannot store a voltage, but migration 08 imports legacy rules
	// straight into these columns, and those rules DID carry voltage values.
	// Writing one directly is the only way to reach the render path (R212).
	_, err := h.pool.Exec(t.Context(),
		`update alarms set voltage_max = 400, voltage_min = 180 where id = $1`, rule.ID)
	require.NoError(t, err)

	got := ca.do(http.MethodGet, "/alarms/"+rule.ID.String(), nil)
	require.Equal(t, http.StatusOK, got.status, string(got.body))
	require.NotContains(t, string(got.body), "voltage_max")
	require.NotContains(t, string(got.body), "voltage_min")

	var after dto.Alarm
	got.json(t, &after)
	require.Nil(t, after.Settings.VoltageMax)
	require.Nil(t, after.Settings.VoltageMin)
}

func TestAlarmRuleWithNoLimitIsRejected(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	// R230, promoted from legacy's browser alert to a 422 with a field code.
	res := ca.do(http.MethodPost, "/alarms", map[string]any{
		"name": "Eksik", "type": "reactive_limit", "is_enabled": true,
		"analyzer_ids": []string{h.fx.AnalyzerA1.String()}, "settings": map[string]any{},
	})
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	require.Contains(t, string(res.body), "settings")
}

func TestAlarmOfAnotherCompanyIs404(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// Company B creates a rule on its own analyzer; company A must not see it.
	other := h.as(seed.E2ECompanyBAdminEmail)
	res := other.do(http.MethodPost, "/alarms", map[string]any{
		"name": "B", "type": "data_communication", "is_enabled": true,
		"analyzer_ids": []string{h.fx.AnalyzerB1.String()},
		"settings":     map[string]any{"communication_threshold_hours": 6},
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var foreign dto.Alarm
	res.json(t, &foreign)

	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/alarms/"+foreign.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/alarms/"+foreign.ID.String()+"/events", nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPost, "/alarms/"+foreign.ID.String()+"/evaluate", nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodDelete, "/alarms/"+foreign.ID.String(), nil).status)

	var page dto.Page[dto.Alarm]
	ca.do(http.MethodGet, "/alarms", nil).json(t, &page)
	require.Empty(t, page.Items)
}

func TestAlarmWriteRefusesAnAnalyzerOutOfScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// The building admin is responsible for A1 only; naming A2 too refuses the
	// whole call with the same 404 an unknown id gets (R213).
	ba := h.as(seed.E2EBuildingAdminEmail)
	res := ba.do(http.MethodPost, "/alarms", commsBody(h, 6, h.fx.AnalyzerA1.String(), h.fx.AnalyzerA2.String()))
	require.Equal(t, http.StatusNotFound, res.status, string(res.body))
}

func TestAlarmDryRunWritesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	// The fixture analyzer has no readings, so last_reading_at is null and the
	// comms alarm cannot decide: a NO-VERDICT, not a breach (R216).
	res := ca.do(http.MethodPost, "/alarms", commsBody(h, 1))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var rule dto.Alarm
	res.json(t, &rule)

	res = ca.do(http.MethodPost, "/alarms/"+rule.ID.String()+"/evaluate", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var out dto.AlarmEvaluation
	res.json(t, &out)
	require.True(t, out.DryRun)
	require.Zero(t, out.NotificationsSent)
	require.Len(t, out.Analyzers, 1)
	require.False(t, out.Analyzers[0].Fired)
	require.Equal(t, []string{"communication_threshold_hours"}, out.Analyzers[0].NoVerdict)

	// Now make the meter stale and evaluate again: it fires, still writing nothing.
	_, err := h.pool.Exec(t.Context(), `update analyzers set last_reading_at = $1 where id = $2`,
		h.clock.Now().Add(-48*time.Hour), h.fx.AnalyzerA1)
	require.NoError(t, err)

	res = ca.do(http.MethodPost, "/alarms/"+rule.ID.String()+"/evaluate", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &out)
	require.True(t, out.Analyzers[0].Fired)
	require.Len(t, out.Analyzers[0].Breaches, 1)
	require.NotEmpty(t, out.Analyzers[0].Breaches[0].Message, "the verdict is rendered, not a bare field name")
	require.Zero(t, out.NotificationsSent)

	// R223: a dry run leaves no event behind, whether it fired or not.
	events := ca.do(http.MethodGet, "/alarms/"+rule.ID.String()+"/events", nil)
	require.Equal(t, http.StatusOK, events.status, string(events.body))
	var page dto.Page[dto.AlarmEvent]
	events.json(t, &page)
	require.Empty(t, page.Items)
}

func TestAlarmEvaluateWithNotifyIsNotSilentlyADryRun(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/alarms", commsBody(h, 1))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var rule dto.Alarm
	res.json(t, &rule)

	// The firing path is not wired in this task. Answering Unavailable is
	// honest; answering 200 with a dry run would tell the operator a
	// notification went out when none did.
	res = ca.do(http.MethodPost, "/alarms/"+rule.ID.String()+"/evaluate?notify=true", nil)
	require.Equal(t, http.StatusServiceUnavailable, res.status, string(res.body))
}

func TestAlarmReadOnlyRolesCannotWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/alarms", commsBody(h, 6))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var rule dto.Alarm
	res.json(t, &rule)

	for _, email := range []string{seed.E2ECompanyReadonlyEmail, seed.E2EBuildingReadonlyEmail} {
		c := h.as(email)
		require.Equal(t, http.StatusOK, c.do(http.MethodGet, "/alarms", nil).status, email)
		require.Equal(t, http.StatusForbidden, c.do(http.MethodPost, "/alarms", commsBody(h, 6)).status, email)
		require.Equal(t, http.StatusForbidden, c.do(http.MethodDelete, "/alarms/"+rule.ID.String(), nil).status, email)
		// alarms.evaluate stops at A and CA, so even a read-only company role
		// cannot start one.
		require.Equal(t, http.StatusForbidden, c.do(http.MethodPost, "/alarms/"+rule.ID.String()+"/evaluate", nil).status, email)
	}
}

func TestAlarmSMSTargetIsStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	body := commsBody(h, 6)
	// R211/D-1: the number is kept so a gateway can be wired later. The notify
	// path is what refuses to pretend it sent anything.
	body["channels"] = []map[string]string{
		{"channel": "email", "target": "ops@example.com"},
		{"channel": "sms", "target": "+905551234567"},
	}
	res := ca.do(http.MethodPost, "/alarms", body)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.Alarm
	res.json(t, &created)
	require.Len(t, created.Channels, 2)

	badNumber := commsBody(h, 6)
	badNumber["channels"] = []map[string]string{{"channel": "sms", "target": "0555 123 45 67"}}
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPost, "/alarms", badNumber).status)
}
