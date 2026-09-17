// Package dto holds the /api/v1 request and response shapes (05 §1): snake_case
// JSON, decimals as strings, dates as YYYY-MM-DD, times in Europe/Istanbul.
package dto

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/swaggest/jsonschema-go"
)

// Istanbul is the zone every response timestamp is rendered in (R161).
var Istanbul = mustLoad("Europe/Istanbul")

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// T renders a timestamp in Istanbul time.
func T(t time.Time) time.Time { return t.In(Istanbul) }

// TP renders an optional timestamp in Istanbul time.
func TP(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.In(Istanbul)
	return &v
}

// Decimal is an exact decimal carried as a JSON string; JSON numbers are refused.
type Decimal struct{ decimal.Decimal }

// D wraps a decimal.
func D(d decimal.Decimal) Decimal { return Decimal{d} }

// DP wraps an optional decimal.
func DP(d *decimal.Decimal) *Decimal {
	if d == nil {
		return nil
	}
	return &Decimal{*d}
}

// MarshalJSON writes the exact decimal as a string.
func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON accepts only a JSON string holding a decimal.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || b[0] != '"' {
		return errors.New("decimal must be a JSON string")
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return errors.New("invalid decimal")
	}
	d.Decimal = v
	return nil
}

// JSONSchema documents Decimal as a string.
func (Decimal) JSONSchema() (jsonschema.Schema, error) {
	var s jsonschema.Schema
	s.AddType(jsonschema.String)
	s.WithPattern(`^-?[0-9]+(\.[0-9]+)?$`)
	s.WithExamples("1234.567890")
	return s, nil
}

// Date is a calendar date, YYYY-MM-DD.
type Date struct{ time.Time }

const dateLayout = "2006-01-02"

// ParseDate parses YYYY-MM-DD as a date in Istanbul.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(dateLayout, s, Istanbul)
	if err != nil {
		return Date{}, errors.New("invalid date")
	}
	return Date{t}, nil
}

// MarshalJSON writes YYYY-MM-DD.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.In(Istanbul).Format(dateLayout)) }

// UnmarshalJSON reads YYYY-MM-DD.
func (d *Date) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return errors.New("date must not be null")
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// UnmarshalText lets Date bind from a query parameter.
func (d *Date) UnmarshalText(b []byte) error {
	v, err := ParseDate(string(b))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// JSONSchema documents Date as a string date.
func (Date) JSONSchema() (jsonschema.Schema, error) {
	var s jsonschema.Schema
	s.AddType(jsonschema.String)
	s.WithFormat("date")
	return s, nil
}

// Error is the error envelope every failure returns (05 §1, R152).
type Error struct {
	Error ErrorBody `json:"error" required:"true"`
}

// ErrorBody is the envelope's content.
type ErrorBody struct {
	Code      string         `json:"code" required:"true"`
	Message   string         `json:"message" required:"true"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

// Empty is a body with no fields ({}).
type Empty struct{}

// JobAccepted is the 202 body of an enqueue.
type JobAccepted struct {
	JobID string `json:"job_id" required:"true"`
}

// CompanyScopeQuery is R190: the admin company selector every authenticated
// route accepts (R139). Handlers never read it — the Scope middleware does —
// so it is only declared to the OpenAPI document and the generated client.
type CompanyScopeQuery struct {
	CompanyID *uuid.UUID `query:"company_id" json:"-"`
}
