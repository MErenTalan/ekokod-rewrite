package tariff_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func TestReadSheetCSVCommaAndSemicolon(t *testing.T) {
	comma := "\xef\xbb\xbfMuhasebe Dönemi,ETSO Kodu,Toplam Kwh,Enerji Bedeli\n202511,X1,\"1,234.50\",99.10\n"
	tbl, err := tariffsvc.ReadSheet("a.CSV", []byte(comma))
	require.NoError(t, err)
	require.Equal(t, []string{"Muhasebe Dönemi", "ETSO Kodu", "Toplam Kwh", "Enerji Bedeli"}, tbl.Header)
	require.Equal(t, [][]string{{"202511", "X1", "1,234.50", "99.10"}}, tbl.Rows)

	semi := "CK Enerji İcmal;;;\nMuhasebe Dönemi;ETSO Kodu;Toplam Kwh;Enerji Bedeli\n202511;X1;1.234,50;99,10\n"
	tbl, err = tariffsvc.ReadSheet("b.csv", []byte(semi))
	require.NoError(t, err)
	require.Equal(t, "Toplam Kwh", tbl.Header[2], "the preamble row is skipped")
	require.Equal(t, [][]string{{"202511", "X1", "1.234,50", "99,10"}}, tbl.Rows)

	_, err = tariffsvc.ReadSheet("c.pdf", []byte("x"))
	require.ErrorIs(t, err, tariffsvc.ErrInvalidRequest)
	_, err = tariffsvc.ReadSheet("d.csv", []byte("a,b\n1,2\n"))
	require.ErrorIs(t, err, tariffsvc.ErrInvalidRequest)
}

func TestReadSheetXLSX(t *testing.T) {
	f := excelize.NewFile()
	rows := [][]any{{"Rapor"}, {"Muhasebe Dönemi", "ETSO Kodu", "Toplam Kwh"}, {"202511", "X1", "12.5"}}
	for i, r := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		require.NoError(t, err)
		require.NoError(t, f.SetSheetRow("Sheet1", cell, &r))
	}
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	tbl, err := tariffsvc.ReadSheet("icmal.xlsx", buf.Bytes())
	require.NoError(t, err)
	require.Equal(t, []string{"Muhasebe Dönemi", "ETSO Kodu", "Toplam Kwh"}, tbl.Header)
	require.Equal(t, [][]string{{"202511", "X1", "12.5"}}, tbl.Rows)
}

// panicTariffs proves Create validates before touching the store: any call panics.
type panicTariffs struct{ store.TariffRepository }

func TestCreateRejectsMissingVatRateBeforeAnyWrite(t *testing.T) {
	svc, err := tariffsvc.New(tariffsvc.Deps{
		Tariffs: panicTariffs{}, Templates: struct{ store.TariffTemplateRepository }{}, Buildings: struct{ store.BuildingRepository }{},
		Analyzers: struct{ store.AnalyzerRepository }{}, Icmal: struct{ store.IcmalRepository }{}, Prices: struct{ store.PriceRepository }{},
		Params: struct {
			store.BillingParameterRepository
		}{}, Clock: clock.NewFake(time.Now()), Log: slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), store.SystemScope(uuid.New()), tariffsvc.Input{Tariff: model.Tariff{}})
	var ve *tariff.ValidationError
	require.True(t, errors.As(err, &ve))
	require.Equal(t, tariff.CodeRequired, ve.Fields["vat_rate"])
}
