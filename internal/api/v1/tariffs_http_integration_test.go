//go:build integration

package v1_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// upload posts a multipart/form-data body with one `file` part — the only
// helper this file needs that the harness does not already have.
func (c *client) upload(t *testing.T, path, fileName string, content []byte) response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, form.Close())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.h.srv.URL+"/api/v1"+path, &body)
	require.NoError(t, err)
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Content-Type", form.FormDataContentType())
	res, err := c.http.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	raw := new(bytes.Buffer)
	_, err = raw.ReadFrom(res.Body)
	require.NoError(t, err)
	return response{status: res.StatusCode, header: res.Header, body: raw.Bytes()}
}

// icmalWorkbook is the real anonymised supplier icmal the domain tests parse;
// uploading a hand-built sheet would prove only that the test can write one.
func icmalWorkbook(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile("../../domain/tariff/icmal/testdata/ck_icmal_2025_11_12_anonymised.csv")
	require.NoError(t, err)
	return content
}

func TestTemplateCrudAndApply(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	body := map[string]any{"name": "Ticari AG", "is_default": true, "tariff": validTariff(uuid.Nil)}
	delete(body["tariff"].(map[string]any), "building_id")
	res := ca.do(http.MethodPost, "/tariff-templates", body)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.TariffTemplate
	res.json(t, &created)
	require.Equal(t, "Ticari AG", created.Name)
	require.True(t, created.IsDefault)
	require.Equal(t, "3.4547", created.Tariff.SingleTimePrice.String(), "the template carries its definition back")

	var page dto.Page[dto.TariffTemplate]
	ca.do(http.MethodGet, "/tariff-templates", nil).json(t, &page)
	require.Len(t, page.Items, 1)

	body["name"] = "Ticari AG v2"
	res = ca.do(http.MethodPatch, "/tariff-templates/"+created.ID.String(), body)
	require.Equal(t, http.StatusOK, res.status, string(res.body))

	// 05 §6: templates are read by A CA CR and written by A CA.
	require.Equal(t, http.StatusForbidden,
		h.as(seed.E2ECompanyReadonlyEmail).do(http.MethodPost, "/tariff-templates", body).status)
	require.Equal(t, http.StatusOK,
		h.as(seed.E2ECompanyReadonlyEmail).do(http.MethodGet, "/tariff-templates", nil).status)
	require.Equal(t, http.StatusNotFound,
		h.as(seed.E2ECompanyBAdminEmail).do(http.MethodGet, "/tariff-templates/"+created.ID.String(), nil).status,
		"another company's template answers like an unknown one")

	apply := map[string]any{"building_ids": []string{h.fx.BuildingA1.String(), h.fx.BuildingA2.String()}, "effective_from": "2026-09-01"}
	res = ca.do(http.MethodPost, "/tariff-templates/"+created.ID.String()+"/apply", apply)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var applied dto.BulkTariffResult
	res.json(t, &applied)
	require.Len(t, applied.TariffIDs, 2)

	var history dto.Page[dto.BulkTariffAssignment]
	ca.do(http.MethodGet, "/buildings/bulk-tariff/history", nil).json(t, &history)
	require.Len(t, history.Items, 1, "an apply is one assignment (R241)")
	require.NotNil(t, history.Items[0].TemplateID)
	require.Equal(t, created.ID, *history.Items[0].TemplateID)
	require.NotNil(t, history.Items[0].CreatedBy)

	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/tariff-templates/"+created.ID.String(), nil).status)
}

func TestBulkCurrentListsEveryBuildingEvenWithoutATariff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	var before dto.Page[dto.BuildingTariffState]
	ca.do(http.MethodGet, "/buildings/bulk-tariff/current", nil).json(t, &before)
	require.NotEmpty(t, before.Items, "a building with no tariff is a row, not an omission")

	assign := map[string]any{"building_ids": []string{h.fx.BuildingA1.String()}, "tariff": validTariff(uuid.Nil)}
	delete(assign["tariff"].(map[string]any), "building_id")
	require.Equal(t, http.StatusOK, ca.do(http.MethodPost, "/buildings/bulk-tariff", assign).status)

	var after dto.Page[dto.BuildingTariffState]
	ca.do(http.MethodGet, "/buildings/bulk-tariff/current", nil).json(t, &after)
	var assigned int
	for _, row := range after.Items {
		if row.BuildingID == h.fx.BuildingA1 {
			require.NotNil(t, row.TariffID)
			assigned++
		}
	}
	require.Equal(t, 1, assigned)
}

func TestIcmalUploadAnalysesWithoutWritingATariff(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.upload(t, "/icmal-imports", "icmal.csv", icmalWorkbook(t))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var imp dto.IcmalImport
	res.json(t, &imp)
	require.Equal(t, "analysed", imp.Status)

	// 02 §8.4: the analysis is stored, and nothing is written to a tariff.
	var page dto.Page[dto.TariffSummaryItem]
	ca.do(http.MethodGet, "/tariffs?building_id="+h.fx.BuildingA1.String(), nil).json(t, &page)
	require.Empty(t, page.Items)

	var again dto.IcmalImport
	ca.do(http.MethodGet, "/icmal-imports/"+imp.ID.String(), nil).json(t, &again)
	require.Equal(t, imp.ID, again.ID)
	require.Equal(t, len(imp.Analyses), len(again.Analyses), "re-reading answers the same question")
}

func TestIcmalUploadRejectsUnreadableFiles(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"empty.csv", nil},
		{"not-a-sheet.csv", []byte("%PDF-1.7\n")},
	} {
		res := ca.upload(t, "/icmal-imports", tc.name, tc.body)
		require.Equal(t, http.StatusUnprocessableEntity, res.status, tc.name+": "+string(res.body))
		require.Equal(t, "validation_failed", res.code(t), tc.name)
	}
}

func TestIcmalUploadTooLarge(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.upload(t, "/icmal-imports", "big.csv", bytes.Repeat([]byte("a"), 11<<20))
	require.Equal(t, http.StatusRequestEntityTooLarge, res.status, string(res.body))
}

func TestIcmalApplyRefusesAnEmptyConfirmationList(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.upload(t, "/icmal-imports", "icmal.csv", icmalWorkbook(t))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var imp dto.IcmalImport
	res.json(t, &imp)

	// R245: nothing is written without an explicit per-building confirmation.
	res = ca.do(http.MethodPost, "/icmal-imports/"+imp.ID.String()+"/apply", map[string]any{"confirmations": []any{}})
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
}

func TestIcmalIsRefusedForAReadonlyAdmin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	res := h.as(seed.E2ECompanyReadonlyEmail).upload(t, "/icmal-imports", "icmal.csv", icmalWorkbook(t))
	require.Equal(t, http.StatusForbidden, res.status, string(res.body))
}

// plant creates one solar plant for the company and returns its id; the e2e
// fixtures have no plant yet (the solar screens arrive in F9).
func (c *client) plant(t *testing.T, name string) uuid.UUID {
	t.Helper()
	res := c.do(http.MethodPost, "/power-plants", map[string]any{"name": name, "plant_kind": "rooftop"})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	res.json(t, &created)
	return created.ID
}

func TestSolarTariffHistoryAndScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	plantID := ca.plant(t, "Çatı GES")

	create := map[string]any{"plant_id": plantID.String(), "effective_from": "2026-01-01",
		"feed_in_tariff": "2.500000", "purchase_price": "3.100000", "currency": "TRY", "notes": "YEKDEM sözleşmesi"}
	res := ca.do(http.MethodPost, "/solar-tariffs", create)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.SolarTariff
	res.json(t, &created)
	require.Equal(t, "2.5", created.FeedInTariff.String())

	var page dto.Page[dto.SolarTariff]
	ca.do(http.MethodGet, "/solar-tariffs?plant_id="+plantID.String(), nil).json(t, &page)
	require.Len(t, page.Items, 1)

	// R244: a plant outside the caller's scope answers like an unknown one.
	require.Equal(t, http.StatusNotFound,
		h.as(seed.E2ECompanyBAdminEmail).do(http.MethodGet, "/solar-tariffs?plant_id="+plantID.String(), nil).status)
	require.Equal(t, http.StatusForbidden,
		h.as(seed.E2ECompanyReadonlyEmail).do(http.MethodPost, "/solar-tariffs", create).status)

	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/solar-tariffs/"+created.ID.String(), nil).status)
	ca.do(http.MethodGet, "/solar-tariffs?plant_id="+plantID.String(), nil).json(t, &page)
	require.Empty(t, page.Items, "a soft-deleted price leaves the history")
}

func TestSolarTariffRefusesANegativeFeedIn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/solar-tariffs", map[string]any{
		"plant_id": ca.plant(t, "Çatı GES").String(), "effective_from": "2026-01-01", "feed_in_tariff": "-1"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
}

func TestNationalScheduleIsPublicToReadAndAdminToWrite(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	row := map[string]any{"effective_from": "2026-01-01", "user_group": "commercial", "voltage_level": "lv",
		"term": "monomial", "energy_price": "3.1", "distribution_price": "2.4", "vat_rate": "20", "source": "EPDK"}

	// 05 §6 lists the read as public: it is what the public bill calculator
	// (F12) will read, and it carries no tenant data at all.
	require.Equal(t, http.StatusOK, h.client(uaChrome).do(http.MethodGet, "/national-tariff-schedule", nil).status)
	require.Equal(t, http.StatusForbidden, h.as(seed.E2ECompanyAdminEmail).do(http.MethodPost, "/national-tariff-schedule", row).status)

	admin := h.as(seed.E2EAdminEmail)
	require.Equal(t, http.StatusCreated, admin.do(http.MethodPost, "/national-tariff-schedule", row).status)

	var page dto.Page[dto.NationalTariff]
	h.client(uaChrome).do(http.MethodGet, "/national-tariff-schedule?user_group=commercial&voltage_level=lv", nil).json(t, &page)
	require.NotEmpty(t, page.Items)

	// The natural key is unique, so publishing the same row twice edits it.
	row["energy_price"] = "3.2"
	require.Equal(t, http.StatusCreated, admin.do(http.MethodPost, "/national-tariff-schedule", row).status)
	var after dto.Page[dto.NationalTariff]
	h.client(uaChrome).do(http.MethodGet, "/national-tariff-schedule?user_group=commercial&voltage_level=lv", nil).json(t, &after)
	require.Len(t, after.Items, len(page.Items), "an upsert edits rather than accumulates")
}
