package v1_test

// fkCovered names every mutation TestForeignKeysAreRefused sends a foreign
// body id to; that test asserts each name here has a case, so they cannot drift.
var fkCovered = map[string]struct{}{
	"alarms.create": {}, "alarms.update": {}, "analyzers.update": {}, "anomaly.check": {},
	"bills.compute": {}, "buildings.create": {}, "buildings.update": {}, "bulk_tariff.assign": {},
	"carbon.activities.create": {}, "carbon.reports.create": {},
	"forecast.monthly": {}, "forecast.run": {}, "forecast.weekly": {},
	"integration_credentials.backfill": {}, "integration_credentials.create": {}, "integration_credentials.update": {},
	"plants.create": {}, "plants.isolar_link": {}, "plants.update": {},
	"reports.generate": {}, "solar_tariffs.create": {},
	"tariff_templates.apply": {}, "tariff_templates.create": {}, "tariff_templates.update": {},
	"tariffs.create": {}, "tariffs.update": {}, "users.create": {}, "users.update": {},
}

// fkExempt is a body-FK mutation without a cross-tenant case, with the reason.
var fkExempt = map[string]string{}
