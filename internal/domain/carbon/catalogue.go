// Package carbon is 02 §9 and 01 §7.16's accounting rules: the activity
// catalogue with its GHG/ISO mapping, emission, pro-rata and the aggregates
// behind the overview and the reports. It is pure.
package carbon

import "github.com/MErenTalan/ekokod-rewrite/internal/domain/model"

// Sub is one activity sub-category with its derived scope and ISO 14064
// category (R301): never entered by the user.
type Sub struct {
	Key   string
	Main  string
	Scope model.CarbonScope
	ISO   string
}

// catalogue is legacy CATEGORY_MAPPING and GHG_ISO_MAPPING verbatim, in
// legacy order (R300, R301; waste is category_6, Q-F1).
var catalogue = []struct {
	main string
	subs []Sub
}{
	{"cat_stationary", []Sub{
		{"sub_space_heating", "cat_stationary", model.CarbonScope1, "category_1"},
		{"sub_process_combustion", "cat_stationary", model.CarbonScope1, "category_1"},
		{"sub_other_combustion", "cat_stationary", model.CarbonScope1, "category_1"},
	}},
	{"cat_mobility", []Sub{
		{"sub_passenger_transport", "cat_mobility", model.CarbonScope1, "category_1"},
		{"sub_business_travel", "cat_mobility", model.CarbonScope3, "category_3"},
		{"sub_employee_commuting", "cat_mobility", model.CarbonScope3, "category_3"},
	}},
	{"cat_logistics", []Sub{
		{"sub_inbound_freight", "cat_logistics", model.CarbonScope3, "category_3"},
		{"sub_outbound_freight", "cat_logistics", model.CarbonScope3, "category_3"},
	}},
	{"cat_gas_emissions", []Sub{
		{"sub_process_emissions", "cat_gas_emissions", model.CarbonScope1, "category_1"},
		{"sub_fugitive_emissions", "cat_gas_emissions", model.CarbonScope1, "category_1"},
	}},
	{"cat_electricity", []Sub{
		{"sub_grid_electricity", "cat_electricity", model.CarbonScope2, "category_2"},
		{"sub_elec_generation", "cat_electricity", model.CarbonScope1, "category_1"},
	}},
	{"cat_external_energy", []Sub{
		{"sub_purchased_hc", "cat_external_energy", model.CarbonScope2, "category_2"},
		{"sub_purchased_steam", "cat_external_energy", model.CarbonScope2, "category_2"},
	}},
	{"cat_products", []Sub{
		{"sub_purchased_goods", "cat_products", model.CarbonScope3, "category_4"},
		{"sub_eol_sold_products", "cat_products", model.CarbonScope3, "category_5"},
		{"sub_capital_goods", "cat_products", model.CarbonScope3, "category_4"},
	}},
	{"cat_waste", []Sub{
		{"sub_waste_disposal", "cat_waste", model.CarbonScope3, "category_6"},
	}},
}

// ISOCategories are the report's ISO 14064 groups, in order.
var ISOCategories = []string{"category_1", "category_2", "category_3", "category_4", "category_5", "category_6"}

// Grid and generation are the sub-categories the daily accrual writes (R311).
const (
	SubGrid       = "sub_grid_electricity"
	SubGeneration = "sub_elec_generation"
)

// Mains lists the main categories in legacy order.
func Mains() []string {
	out := make([]string, len(catalogue))
	for i, c := range catalogue {
		out[i] = c.main
	}
	return out
}

// Subs lists a main category's sub-categories; nil for an unknown one.
func Subs(main string) []Sub {
	for _, c := range catalogue {
		if c.main == main {
			return append([]Sub(nil), c.subs...)
		}
	}
	return nil
}

// SubByKey resolves a sub-category key.
func SubByKey(key string) (Sub, bool) {
	for _, c := range catalogue {
		for _, s := range c.subs {
			if s.Key == key {
				return s, true
			}
		}
	}
	return Sub{}, false
}
