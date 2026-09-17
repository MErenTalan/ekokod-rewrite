package billing_test

import "testing"

// TestF4Acceptance lists every §F4 acceptance bullet (09-implementation-plan.md)
// with the test that proves it; `go test ./internal/domain/billing -run
// TestF4Acceptance -v` shows them all. Bullets proven outside this package:
//
//	bullet                                   package                          test
//	tariff without vat_rate rejected         internal/domain/tariff           TestValidateRejectsMissingVatRate
//	                                         internal/service/tariff          TestCreateRejectsMissingVatRateBeforeAnyWrite
//	PTF missing hours flag, not issue        internal/domain/tariff           TestPriceHourlyMissingHoursOverToleranceIsIncomplete
//	                                         internal/service/billing (integ) TestGeneratePTFMissingHoursFlagsInsteadOfIssuing
//	KBK coefficients independent             internal/domain/tariff           TestPricePeriodAverageDerivesEachCoefficientIndependently
//	unresolved anomaly blocks issue          internal/service/billing (integ) TestGenerateBlocksOnUnresolvedAnomaly
//	icmal round trip, back-calc within 2 %   internal/domain/tariff/icmal     TestParseRealFixtureRowCountAndTotals, TestAnalyseRealFixtureWithinTwoPercent
//	PDF Turkish glyphs, byte-stable          internal/render/invoicepdf       TestRenderContainsTurkishGlyphs, TestRenderIsByteStable
//	real-invoice comparison                  internal/cli                     TestCompareInvoiceMatchesAnonymisedIcmalRowExactly (anonymised icmal row; PO invoices OPEN)
func TestF4Acceptance(t *testing.T) {
	t.Run("GoldenFilesForEveryRule", TestGolden)
	t.Run("DistributionChargedOnTotalConsumption", TestDistributionChargedOnTotalNotLowTier)
	t.Run("MultiTimeNeverAveragedNeverTiered", TestMultiTimeNeverTieredNeverAveraged)
	t.Run("NullKbkCoefficientProducesNoLine", TestPTFNilCoefficientProducesNoLine)
	t.Run("BuildingInvoiceIsNotSumOfAnalyzerInvoicesWhenTiered", TestBuildingInvoiceIsNotSumOfAnalyzerInvoicesWhenTiered)
	t.Run("ReactiveExemptions", TestReactiveExemptionsProduceNoLines)
	t.Run("IcmalRowAssemblyMatchesSupplier", TestIcmalRowAssemblyMatchesSupplierToTheKurus)
	t.Run("ComputeProperty", TestComputePropertyInvariants)
}
