package aril

import "encoding/json"

// authRequest is authentication's POST body (06 §4 Endpoints table):
// {UserCode, Password}.
type authRequest struct {
	UserCode string `json:"UserCode"`
	Password string `json:"Password"`
}

// tokenObject is authentication's object-shaped success response: 06 §4
// "the token comes back as a bare string or as {access_token}" — this is
// the second of the two shapes decodeToken tries.
type tokenObject struct {
	AccessToken string `json:"access_token"`
}

// subscriptionsRequest is analyzers_list's POST body (06 §4 Endpoints
// table): {PageNumber, PageSize}.
type subscriptionsRequest struct {
	PageNumber int `json:"PageNumber"`
	PageSize   int `json:"PageSize"`
}

// subscriptionsResponse is analyzers_list's response envelope. ErrorCode is
// *json.Number and ResultList is *[]json.RawMessage, not their bare forms,
// for the same reason GridBox's ResultStatus is a pointer (adapter review
// pattern 6): a response whose top-level shape is not the expected
// envelope at all — the key genuinely absent because it is missing, the
// whole body is `null`, or the key itself is `null` — must be
// distinguishable from an explicit, legitimate zero/empty value.
//
// 06 §4 does not name analyzers_list's error-signalling field explicitly;
// this adapter follows task-8-brief.md's own vocabulary ("A response with
// ErrorCode != 0 on a data call → ErrUpstreamUnavailable") and applies the
// same {ErrorCode, <list>} envelope shape uniformly to every ARIL POST
// endpoint. Flagged as a spec gap in the task report — 06 does not give
// ARIL's exact envelope field names the way it does for GridBox's
// {ResultStatus,ResultObject}.
type subscriptionsResponse struct {
	ErrorCode  *json.Number       `json:"ErrorCode"`
	ResultList *[]json.RawMessage `json:"ResultList"`
}

// subscriptionWire is one analyzers_list row, exactly as 06 §4's
// "Subscription mapping" table names ARIL's own fields. Every field but
// SubscriptionSerno is a pointer: DiscoverMeteringPoints has no per-row
// Warning channel (unlike FetchResult), so a present-but-unparseable
// optional field is mapped best-effort (left nil) rather than failing the
// whole row — only a missing/blank SubscriptionSerno (the identity field
// integration.MeteringPoint cannot be built without) drops a row.
//
// SubscriptionSerno is a string, not a json.Number: 06 §4 does not state
// its wire type, and fake's sanitiser (installationKeys) requires every
// ARIL fixture's "SubscriptionSerno" value to be an FX-placeholder string
// (the same convention OSOS's instalationNumber and GridBox's wiringNo
// fixtures use for their own installation identifiers) — consistent with
// treating it the same as every other provider's installation-identifier
// field, never specifically as a numeric type this adapter would have to
// invent a distinct convention for.
type subscriptionWire struct {
	SubscriptionSerno string       `json:"SubscriptionSerno"`
	Title             *string      `json:"Title"`
	Address           *string      `json:"Address"`
	MeterSerial       *string      `json:"MeterSerial"`
	MeterBrand        *string      `json:"MeterBrand"`
	Multiplier        *json.Number `json:"Multiplier"`
	InstalledPower    *json.Number `json:"InstalledPower"`
	AccordPower       *json.Number `json:"AccordPower"`
	Etso              *string      `json:"Etso"`
	// LastEndexDate/LastProfileDate: 06 §4 documents these as "provider
	// high-water marks" but never states their wire format. This adapter
	// assumes the same 14-digit yyyyMMddHHmmss encoding 06 §4 documents
	// for LoadProfiles[].ProfileDate — the one date format ARIL's API is
	// documented to use anywhere — rather than inventing a new one.
	// Flagged as a spec gap in the task report; a value that fails to
	// parse under that assumption is simply left out of
	// MeteringPoint.ProviderHighWater (best-effort, see the struct doc
	// above), never a hard discovery failure.
	LastEndexDate   *json.Number `json:"LastEndexDate"`
	LastProfileDate *json.Number `json:"LastProfileDate"`
	DefinitionType  *json.Number `json:"DefinitionType"`
}

// registers is the six-register set 06 §4's "Load profile mapping" table
// lists. owner_consumptions' LoadProfiles rows and current_endexes/
// end_of_month_endexes rows all report index/interval data with this same
// field spelling (06 gives the field set once; embedding here rather than
// duplicating it keeps loadProfileRow and endexRow from drifting apart).
// Every value is *string, never *json.Number: task-8-brief.md's own
// mutation example shows TSum arriving quoted ("TSum \"12.5\" x 40 ->
// 500"), and normalize.OptionalNumber (never Multiply-on-zero) is what
// converts a provider numeric string to *decimal.Decimal while preserving
// removed-behaviour 21 (absent/""/"-"/"null" -> nil, never zero).
type registers struct {
	TSum                  *string `json:"TSum"`
	ReactiveInductive     *string `json:"ReactiveInductive"`
	ReactiveCapasitive    *string `json:"ReactiveCapasitive"` // sic: ARIL's own spelling (06 §4)
	TSumOut               *string `json:"TSumOut"`
	ReactiveInductiveOut  *string `json:"ReactiveInductiveOut"`
	ReactiveCapasitiveOut *string `json:"ReactiveCapasitiveOut"` // sic
}

// consumptionsRequest is owner_consumptions' POST body (task-8-brief.md
// item 3, verbatim field set).
type consumptionsRequest struct {
	OwnerSerno          string `json:"OwnerSerno"`
	StartDate           string `json:"StartDate"`
	EndDate             string `json:"EndDate"`
	IncludeLoadProfiles bool   `json:"IncludeLoadProfiles"`
	OwnerType           int16  `json:"OwnerType"`
	WithoutMultiplier   bool   `json:"WithoutMultiplier"`
	MergeResult         bool   `json:"MergeResult"`
}

// consumptionsResponse is owner_consumptions' response envelope.
// LoadProfiles is *[]json.RawMessage for the same missing/null-vs-empty
// reason subscriptionsResponse.ResultList is.
type consumptionsResponse struct {
	ErrorCode    *json.Number       `json:"ErrorCode"`
	LoadProfiles *[]json.RawMessage `json:"LoadProfiles"`
}

// loadProfileRow is one owner_consumptions LoadProfiles[] entry (06 §4
// "Load profile mapping"). ProfileDate is the documented 14-digit number.
type loadProfileRow struct {
	ProfileDate json.Number `json:"ProfileDate"`
	registers
}

// endexesRequest is current_endexes' POST body (task-8-brief.md item 4,
// verbatim field set). 06 §4 does not separately document
// end_of_month_endexes' request body ("period-scoped index snapshots" is
// all it says) — this adapter sends the same shape for both endpoints, the
// least-invented choice given 06's silence. Flagged as a spec gap in the
// task report.
type endexesRequest struct {
	OwnerSerno     string `json:"OwnerSerno"`
	StartDate      string `json:"StartDate"`
	EndDate        string `json:"EndDate"`
	DefinitionType int16  `json:"DefinitionType"`
	EndexDirection int    `json:"EndexDirection"`
}

// endexesResponse is current_endexes' and end_of_month_endexes' response
// envelope. 06 §4 names neither endpoint's response field explicitly
// (unlike analyzers_list's documented "ResultList"); this adapter reuses
// "ResultList" for consistency with the one documented ARIL list envelope.
// Flagged as a spec gap in the task report.
type endexesResponse struct {
	ErrorCode  *json.Number       `json:"ErrorCode"`
	ResultList *[]json.RawMessage `json:"ResultList"`
}

// endexRow is one current_endexes/end_of_month_endexes row: the same
// six-register index set plus MaxDemand/MaxDemandDate (06 §4 "Max demand":
// "current_endexes returns MaxDemand and MaxDemandDate per record").
// end_of_month_endexes rows are decoded with this same type; a row that
// carries no MaxDemand simply leaves those two fields nil.
type endexRow struct {
	ProfileDate json.Number `json:"ProfileDate"`
	registers
	MaxDemand     *string      `json:"MaxDemand"`
	MaxDemandDate *json.Number `json:"MaxDemandDate"`
}
