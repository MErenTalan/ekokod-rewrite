package kit

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
)

// CompanyParam is accepted on every route: R139's admin company selector.
const CompanyParam = "company_id"

var (
	validate     *validator.Validate
	validateOnce sync.Once
	uuidType     = reflect.TypeOf(uuid.UUID{})
	textType     = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

func validatorInstance() *validator.Validate {
	validateOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
		validate.RegisterTagNameFunc(fieldName)
		// Coordinates are numeric(9,6): out of range they failed the insert with a 500.
		_ = validate.RegisterValidation("latitude", decimalWithin(90))
		_ = validate.RegisterValidation("longitude", decimalWithin(180))
	})
	return validate
}

// wireName drops the root type and embedded struct names (capitalised Go
// names) from a validator namespace, leaving the JSON path the client sent.
func wireName(namespace string) string {
	parts := strings.Split(namespace, ".")[1:]
	out := parts[:0]
	for _, p := range parts {
		if p != "" && p[0] >= 'A' && p[0] <= 'Z' {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, ".")
}

func decimalWithin(limit int64) validator.Func {
	return func(fl validator.FieldLevel) bool {
		d, ok := fl.Field().Interface().(dto.Decimal)
		return !ok || d.Abs().LessThanOrEqual(decimal.NewFromInt(limit))
	}
}

// fieldName is the wire name of a field: json, then query, then path tag.
func fieldName(f reflect.StructField) string {
	if n, _, _ := strings.Cut(f.Tag.Get("json"), ","); n != "" && n != "-" {
		return n
	}
	if n := f.Tag.Get("query"); n != "" {
		return n
	}
	return f.Tag.Get("path")
}

// Bind fills dst (a pointer to struct) from the JSON body, the query string and
// the chi path, rejects unknown query parameters and runs `validate` tags (R154).
func Bind(r *http.Request, dst any) error {
	v := reflect.ValueOf(dst).Elem()
	if hasBodyFields(v.Type()) {
		if err := decodeBody(r, dst); err != nil {
			return err
		}
	}
	allowed := map[string]bool{CompanyParam: true}
	collectQueryNames(v.Type(), allowed)
	query := r.URL.Query()
	for name := range query {
		if !allowed[name] {
			return ErrUnknownParameter.WithParams(map[string]any{"parameter": name})
		}
	}
	if err := bindFields(r, v, query); err != nil {
		return err
	}
	if err := validatorInstance().Struct(dst); err != nil {
		var verrs validator.ValidationErrors
		if !errors.As(err, &verrs) {
			return err
		}
		details := map[string]any{}
		for _, fe := range verrs {
			name := wireName(fe.Namespace())
			codes, _ := details[name].([]string)
			details[name] = append(codes, fe.Tag())
		}
		return perr.Validation.WithParams(details)
	}
	return nil
}

func hasBodyFields(t reflect.Type) bool {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			if hasBodyFields(f.Type) {
				return true
			}
			continue
		}
		if n, _, _ := strings.Cut(f.Tag.Get("json"), ","); f.IsExported() && n != "-" && f.Tag.Get("query") == "" && f.Tag.Get("path") == "" {
			return true
		}
	}
	return false
}

func decodeBody(r *http.Request, dst any) error {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return ErrBodyTooLarge
		}
		return ErrInvalidBody
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if !json.Valid(raw) {
			return ErrInvalidBody.WithParams(map[string]any{"reason": "malformed JSON"})
		}
		// A value its field cannot hold (a Turkish "12,5", a bad date) is the
		// operator's to fix: 422 naming every such field so the form marks it.
		p := findBodyProblems(raw, reflect.TypeOf(dst))
		if len(p.values) > 0 {
			return perr.Validation.WithParams(p.values)
		}
		if len(p.unknown) > 0 {
			return ErrInvalidBody.WithParams(map[string]any{"reason": "unknown field", "field": p.unknown[0]})
		}
		return ErrInvalidBody.WithParams(map[string]any{"reason": "malformed JSON"})
	}
	if dec.More() {
		return ErrInvalidBody.WithParams(map[string]any{"reason": "trailing data"})
	}
	return nil
}

func collectQueryNames(t reflect.Type, into map[string]bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			collectQueryNames(f.Type, into)
		}
		if n := f.Tag.Get("query"); n != "" {
			into[n] = true
		}
	}
}

func bindFields(r *http.Request, v reflect.Value, query map[string][]string) error {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		fv := v.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			if err := bindFields(r, fv, query); err != nil {
				return err
			}
			continue
		}
		if name := f.Tag.Get("path"); name != "" {
			// chi routes on RawPath when the URL has one, so the segment can
			// still be escaped (a job id's colons arrive as %3A).
			raw, err := url.PathUnescape(chi.URLParam(r, name))
			if err != nil {
				return perr.NotFound
			}
			if err := setValue(fv, []string{raw}); err != nil {
				return perr.NotFound // a malformed id names nothing
			}
		}
		if name := f.Tag.Get("query"); name != "" {
			values, ok := query[name]
			if !ok {
				continue
			}
			if err := setValue(fv, values); err != nil {
				return ErrInvalidParameters.WithParams(map[string]any{name: []string{"invalid"}})
			}
		}
	}
	return nil
}

func setValue(fv reflect.Value, values []string) error {
	if fv.Kind() == reflect.Slice && fv.Type() != reflect.TypeOf(json.RawMessage{}) {
		var parts []string
		for _, v := range values {
			for _, p := range strings.Split(v, ",") {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
		}
		out := reflect.MakeSlice(fv.Type(), len(parts), len(parts))
		for i, p := range parts {
			if err := setScalar(out.Index(i), p); err != nil {
				return err
			}
		}
		fv.Set(out)
		return nil
	}
	if len(values) != 1 {
		return errors.New("repeated scalar")
	}
	if fv.Kind() == reflect.Pointer {
		ptr := reflect.New(fv.Type().Elem())
		if err := setScalar(ptr.Elem(), values[0]); err != nil {
			return err
		}
		fv.Set(ptr)
		return nil
	}
	return setScalar(fv, values[0])
}

func setScalar(fv reflect.Value, raw string) error {
	if fv.Addr().Type().Implements(textType) {
		return fv.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
	}
	if fv.Type() == uuidType {
		id, err := uuid.Parse(raw)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(id))
		return nil
	}
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, fv.Type().Bits())
		if err != nil {
			return err
		}
		fv.SetInt(n)
	default:
		return errors.New("unsupported field kind " + fv.Kind().String())
	}
	return nil
}
