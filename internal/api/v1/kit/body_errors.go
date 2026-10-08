package kit

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
)

var (
	decimalType   = reflect.TypeOf(dto.Decimal{})
	dateType      = reflect.TypeOf(dto.Date{})
	unmarshalType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
)

// bodyProblems walks a body that failed to decode against its target type and
// names each value that does not fit (path → code) and each unknown field, in
// the validator's path format (`children[0].rate`), so a form can mark them.
type bodyProblems struct {
	values  map[string]any
	unknown []string
}

func findBodyProblems(raw []byte, t reflect.Type) bodyProblems {
	p := bodyProblems{values: map[string]any{}}
	p.walk(raw, t, "")
	return p
}

func (p *bodyProblems) walk(raw []byte, t reflect.Type, path string) {
	pointer := t.Kind() == reflect.Pointer
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	custom := reflect.PointerTo(t).Implements(unmarshalType)
	// null leaves a pointer nil and a plain value untouched; only a custom
	// unmarshaler behind a non-pointer sees it (and may refuse it).
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && (pointer || !custom) {
		return
	}
	switch {
	case !custom && t.Kind() == reflect.Struct:
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) != nil {
			p.values[path] = []string{"invalid"}
			return
		}
		fields := jsonFields(t)
		for key, value := range obj {
			f, ok := lookupField(fields, key)
			if !ok {
				p.unknown = append(p.unknown, join(path, key))
				continue
			}
			p.walk(value, f, join(path, key))
		}
	case !custom && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) && t.Elem().Kind() != reflect.Uint8:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			p.values[path] = []string{"invalid"}
			return
		}
		for i, item := range items {
			p.walk(item, t.Elem(), path+"["+strconv.Itoa(i)+"]")
		}
	default:
		if json.Unmarshal(raw, reflect.New(t).Interface()) != nil {
			p.values[path] = []string{codeFor(t)}
		}
	}
}

func codeFor(t reflect.Type) string {
	switch t {
	case decimalType:
		return "decimal"
	case dateType:
		return "date"
	case uuidType:
		return "uuid"
	}
	return "invalid"
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// jsonFields maps wire names to types the way encoding/json does: tagged or Go
// names, untagged embedded structs promoted, `-` skipped.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if f.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			for k, v := range jsonFields(ft) {
				if _, taken := out[k]; !taken {
					out[k] = v
				}
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = f.Type
	}
	return out
}

// lookupField matches exactly first, then case-insensitively as encoding/json does.
func lookupField(fields map[string]reflect.Type, key string) (reflect.Type, bool) {
	if t, ok := fields[key]; ok {
		return t, true
	}
	for name, t := range fields {
		if strings.EqualFold(name, key) {
			return t, true
		}
	}
	return nil, false
}
