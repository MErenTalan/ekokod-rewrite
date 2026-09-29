package integration

import "strings"

// EndpointKeys is the endpoint-template keys a provider's adapter reads from
// integration_definitions.endpoints. Required keys are enforced when a
// definition is saved, so a typo'd key fails there instead of in every fetch.
type EndpointKeys struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

// EndpointCatalog is the one list of endpoint keys per provider; the adapters'
// tests pin their own required keys to it and the admin UI renders it.
var EndpointCatalog = map[Provider]EndpointKeys{
	ProviderOSOS: {Required: []string{"authentication", "analyzers_list", "hourly_values", "energy_values"}},
	ProviderGridBox: {
		Required: []string{"token", "last_success_date", "last_endex", "load_profiles", "endexes"},
		Optional: []string{"energy_values"},
	},
	ProviderARIL: {
		Required: []string{"authentication", "analyzers_list", "owner_consumptions"},
		Optional: []string{"current_endexes", "end_of_month_endexes"},
	},
	ProviderPM5340: {},
	ProviderISolar: {
		Required: []string{"gateway", "token", "refresh_token", "authorize_origin", "cloud_id"},
		Optional: []string{
			"query_power_station_list", "get_power_station_detail", "get_device_list_by_ps_id",
			"get_device_real_time_data", "get_fault_alarm_info", "get_device_point_minute_data_list",
			"get_power_station_point_minute_data_list", "get_device_point_day_month_year_data_list",
			"get_power_station_point_day_month_year_data_list",
		},
	},
}

// MissingEndpointKeys returns p's required keys that endpoints lacks or leaves blank.
func MissingEndpointKeys(p Provider, endpoints map[string]string) []string {
	var missing []string
	for _, k := range EndpointCatalog[p].Required {
		if v, ok := endpoints[k]; !ok || strings.TrimSpace(v) == "" {
			missing = append(missing, k)
		}
	}
	return missing
}
