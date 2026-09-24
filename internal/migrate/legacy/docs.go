package legacy

import (
	"fmt"

	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// asDoc accepts both embedded-document forms the driver produces (bson.D from
// raw/extended JSON, bson.M from literals); anything else is nil.
func asDoc(v any) bson.M {
	switch d := v.(type) {
	case bson.M:
		return d
	case bson.D:
		m := make(bson.M, len(d))
		for _, e := range d {
			m[e.Key] = e.Value
		}
		return m
	case map[string]any:
		return d
	}
	return nil
}

// str is a document field as a string ("" when absent or not a string).
func str(doc bson.M, key string) string {
	s, _ := doc[key].(string)
	return s
}

// num reads a legacy numeric field stored as a number or a string.
func num(v any) (*decimal.Decimal, error) {
	switch n := v.(type) {
	case nil:
		return nil, nil //nolint:nilnil // absent is null
	case int32:
		d := decimal.NewFromInt(int64(n))
		return &d, nil
	case int64:
		d := decimal.NewFromInt(n)
		return &d, nil
	case float64:
		d := decimal.NewFromFloat(n)
		return &d, nil
	case bson.Decimal128:
		d, err := decimal.NewFromString(n.String())
		return &d, err
	case string:
		return ParseNumber(n)
	}
	return nil, fmt.Errorf("%w: %T", ErrBadNumber, v)
}

// rawToM decodes one source document.
func rawToM(raw bson.Raw) (bson.M, error) {
	var m bson.M
	return m, bson.Unmarshal(raw, &m)
}
