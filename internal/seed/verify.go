package seed

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// FactorDiff is R317's comparison of the platform rows with the shipped catalogue.
type FactorDiff struct {
	Total, Matched          int
	Missing, Extra, Changed []string
}

// OK reports a complete, unchanged load.
func (d FactorDiff) OK() bool { return len(d.Missing)+len(d.Extra)+len(d.Changed) == 0 }

func (d FactorDiff) String() string {
	s := fmt.Sprintf("emission factors: %d/%d match", d.Matched, d.Total)
	for _, part := range []struct {
		name string
		keys []string
	}{{"missing", d.Missing}, {"extra", d.Extra}, {"changed", d.Changed}} {
		if len(part.keys) > 0 {
			s += fmt.Sprintf("; %s [%s]", part.name, strings.Join(part.keys, " "))
		}
	}
	return s
}

// CompareFactors compares keys and base factors (numerically: the column
// pads to 8 dp).
func CompareFactors(shipped []EmissionFactor, stored map[string]decimal.Decimal) FactorDiff {
	d := FactorDiff{Total: len(shipped)}
	seen := map[string]bool{}
	for _, f := range shipped {
		seen[f.Key] = true
		v, ok := stored[f.Key]
		switch {
		case !ok:
			d.Missing = append(d.Missing, f.Key)
		case !v.Equal(f.BaseFactor):
			d.Changed = append(d.Changed, f.Key)
		default:
			d.Matched++
		}
	}
	for k := range stored {
		if !seen[k] {
			d.Extra = append(d.Extra, k)
		}
	}
	sort.Strings(d.Missing)
	sort.Strings(d.Extra)
	sort.Strings(d.Changed)
	return d
}

// VerifyEmissionFactors reads the platform rows (company_id null) and
// compares them with the embedded catalogue (R317).
func VerifyEmissionFactors(ctx context.Context, pool *pgxpool.Pool) (FactorDiff, error) {
	shipped, err := EmissionFactors()
	if err != nil {
		return FactorDiff{}, err
	}
	rows, err := pool.Query(ctx, `select key, base_factor::text from emission_factors where company_id is null`)
	if err != nil {
		return FactorDiff{}, fmt.Errorf("verify emission factors: %w", err)
	}
	defer rows.Close()
	stored := map[string]decimal.Decimal{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return FactorDiff{}, err
		}
		v, err := decimal.NewFromString(value)
		if err != nil {
			return FactorDiff{}, fmt.Errorf("emission factor %s: %w", key, err)
		}
		stored[key] = v
	}
	if err := rows.Err(); err != nil {
		return FactorDiff{}, err
	}
	return CompareFactors(shipped, stored), nil
}
