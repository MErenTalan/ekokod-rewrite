package gridbox

import "encoding/json"

// envelope is GridBox's response wrapper (06 §3 rule 2): every endpoint
// except token responds { ResultStatus, ResultObject, ... }. ResultStatus
// == 1 is success; a present-but-non-1 ResultStatus is an upstream-reported
// failure that must surface as ErrUpstreamUnavailable, never be treated as
// an empty result (R25). ResultStatus is a *int, not int, specifically so a
// response whose top-level shape is not even the expected envelope at all
// (an empty object, a bare JSON null, or a wrong/absent key) — where
// ResultStatus decodes as genuinely ABSENT rather than an explicit business
// value — is distinguishable from a legitimate "ResultStatus: 0" and
// surfaces as ErrMalformedPayload instead (adapter review pattern 6: "a 200
// response whose top-level key is missing or null is ErrMalformedPayload,
// never an empty success"). ResultObject is left as json.RawMessage so its
// shape (a single object for last_success_date/last_endex, a list for the
// windowed endpoints) can be decoded per-endpoint only once ResultStatus is
// known good.
type envelope struct {
	ResultStatus *int            `json:"ResultStatus"`
	ResultObject json.RawMessage `json:"ResultObject"`
}

// tokenResponse is the token endpoint's success shape: an OAuth2
// password-grant access token (06 §3 step 1; legacy
// gridbox/refresh/route.ts:93-113). Unlike every other GridBox endpoint,
// token is a plain OAuth2 exchange, not the {ResultStatus,ResultObject}
// envelope — 06 §3's "every response is wrapped" describes the five
// wiring-number endpoints that follow the Flow section's step 2, not the
// authentication step that precedes it. R37 (verify in F14): this reading
// of "every response is wrapped" as excluding the token step has not been
// confirmed against the real provider or legacy TS source; if 06 intends
// the token response to also be envelope-wrapped, this needs a fix.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

// last_success_date's ResultObject is a BARE STRING, not an object (R50,
// re-ruling round-1's lastSuccessDateResponse object type): legacy
// gridbox/refresh/route.ts:138 types the whole response
// `GridResponse<string>` and assigns `a.lastLoadProfileDate =
// res.data.ResultObject` directly — ResultObject IS the ISO-ish date
// string, never a wrapper object. setup/route.ts:318 (also read for R50)
// agrees. Round-1's object shape made decodeLastSuccessDate error against
// the real provider's actual envelope, which turned every GridBox
// FetchReadings into an unconditional ErrMalformedPayload/SkipRetry.
// decodeLastSuccessDate below therefore decodes ResultObject straight into
// a string — no object fallback: legacy shows only the bare-string shape,
// never both, so there is nothing to be tolerant of (R50's own instruction
// is to keep an object fallback only when legacy shows both shapes).

// register carries every field 06 §3's field-mapping table lists, exactly
// as GridBox names them. last_endex, load_profiles, endexes and
// energy_values ResultObjects all share this shape — any one payload
// populates whichever subset applies to it, leaving the rest absent
// (removed-behaviour 21: absent means nil, never zero).
//
// Every numeric field is *json.Number, never float64 (arch guard
// TestIntegrationTreesDoNotParseFloats) — normalize.JSONNumber converts to
// decimal.Decimal digit-for-digit.
type register struct {
	ActiveEndex                  *json.Number `json:"ActiveEndex"`
	ActiveEndexWithMultiplier    *json.Number `json:"ActiveEndexWithMultiplier"`
	ActiveEndexOut               *json.Number `json:"ActiveEndexOut"`
	ActiveEndexOutWithMultiplier *json.Number `json:"ActiveEndexOutWithMultiplier"`

	RI                         *json.Number `json:"RI"`
	ReactiveInductiveEndex     *json.Number `json:"ReactiveInductiveEndex"`
	RC                         *json.Number `json:"RC"`
	ReactiveCapacitiveEndex    *json.Number `json:"ReactiveCapacitiveEndex"`
	RIOut                      *json.Number `json:"RIOut"`
	ReactiveInductiveEndexOut  *json.Number `json:"ReactiveInductiveEndexOut"`
	RCOut                      *json.Number `json:"RCOut"`
	ReactiveCapacitiveEndexOut *json.Number `json:"ReactiveCapacitiveEndexOut"`

	T1               *json.Number `json:"T1"`
	T1Endex          *json.Number `json:"T1Endex"`
	T1WithMultiplier *json.Number `json:"T1WithMultiplier"`
	T2               *json.Number `json:"T2"`
	T2Endex          *json.Number `json:"T2Endex"`
	T2WithMultiplier *json.Number `json:"T2WithMultiplier"`
	T3               *json.Number `json:"T3"`
	T3Endex          *json.Number `json:"T3Endex"`
	T3WithMultiplier *json.Number `json:"T3WithMultiplier"`

	T1Out *json.Number `json:"T1Out"`
	T2Out *json.Number `json:"T2Out"`
	T3Out *json.Number `json:"T3Out"`

	MaxDemand               *json.Number `json:"MaxDemand"`
	MaxDemandWithMultiplier *json.Number `json:"MaxDemandWithMultiplier"`

	// Timestamp candidates, in the order 06 §3's mapping table lists them.
	// Exactly which one a given endpoint populates depends on the call:
	// load_profiles uses ProfileDateTime/ProfileDate, endexes uses
	// EndexDate, energy_values and last_endex use ReadDate — but a payload
	// is never required to use only "its" field, so resolveTimestamp tries
	// all four in order.
	ProfileDateTime *string `json:"ProfileDateTime"`
	ProfileDate     *string `json:"ProfileDate"`
	EndexDate       *string `json:"EndexDate"`
	ReadDate        *string `json:"ReadDate"`

	MeterSerialNumber *string `json:"MeterSerialNumber"`
}

// LastEndex is the last_endex endpoint's ResultObject: the latest index
// snapshot, which is both a current_index reading and (06 §3 "Multiplier
// resolution" priority 1) the primary source of the meter multiplier.
// Exported, with LoadProfileRow, so ResolveMultiplier's own tests can build
// one directly without going through JSON.
type LastEndex struct {
	register
	Multiplier *json.Number `json:"Multiplier"`
}

// LoadProfileRow is one row of the load_profiles endpoint's ResultObject
// list, and (06 §3 "Multiplier resolution" priority 2) the fallback source
// of the meter multiplier when no last_endex.Multiplier is present.
type LoadProfileRow struct {
	register
}

// endexes' and energy_values' ResultObject lists are decoded straight into
// []register (source.go's decodeRegisterRows) — daily/billing index
// snapshots and reset/change events share exactly register's fields, so no
// separate named row type is needed for them the way LastEndex and
// LoadProfileRow are (both exported for ResolveMultiplier's direct tests).
