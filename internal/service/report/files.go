package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportpdf"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/reportxlsx"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrNotReady is a report that has no figures yet (R265).
var ErrNotReady = perr.New("report_not_ready", perr.Conflict.HTTPStatus, "errors.reports.notReady")

// The two file formats.
const (
	FormatPDF   = "pdf"
	FormatExcel = "excel"
)

// Files stores and reads a report's PDF and workbook (R265).
type Files struct {
	Root      string
	Companies store.CompanyRepository
	Buildings store.BuildingRepository
}

func (f Files) dir(companyID uuid.UUID) string {
	return filepath.Join(f.Root, "reports", companyID.String())
}

func ext(format string) string {
	if format == FormatPDF {
		return ".pdf"
	}
	return ".xlsx"
}

// FileName is the name a download and an attachment carry.
func FileName(rp model.Report, format string) string { return "rapor-" + rp.Period + ext(format) }

// payload decodes a stored report; an empty one is not ready.
func payload(rp model.Report) (domain.Payload, error) {
	var p domain.Payload
	if err := json.Unmarshal(rp.Payload, &p); err != nil || (p.Monthly == nil && p.Yearly == nil) {
		return domain.Payload{}, ErrNotReady
	}
	return p, nil
}

// Read returns the stored Turkish file when it is there and inside the
// company's directory; otherwise it renders from the payload. It never writes.
func (f Files) Read(ctx context.Context, rp model.Report, format, locale string) ([]byte, error) {
	if locale != "en" {
		locale = "tr"
		stored := rp.PdfPath
		if format == FormatExcel {
			stored = rp.ExcelPath
		}
		if raw, ok := f.stored(rp.CompanyID, stored); ok {
			return raw, nil
		}
	}
	p, err := payload(rp)
	if err != nil {
		return nil, err
	}
	return f.render(ctx, rp, p, format, locale)
}

// stored reads a recorded path only when it resolves inside the company's
// report directory: a path in the row is data, never trusted as a location.
func (f Files) stored(companyID uuid.UUID, path *string) ([]byte, bool) {
	if path == nil {
		return nil, false
	}
	dir := f.dir(companyID) + string(filepath.Separator)
	clean := filepath.Clean(*path)
	if !strings.HasPrefix(clean, dir) {
		return nil, false
	}
	raw, err := os.ReadFile(clean)
	if err != nil {
		return nil, false
	}
	return raw, true
}

func (f Files) render(ctx context.Context, rp model.Report, p domain.Payload, format, locale string) ([]byte, error) {
	sc := store.SystemScope(rp.CompanyID)
	company, err := f.Companies.Get(ctx, sc, rp.CompanyID)
	if err != nil {
		return nil, err
	}
	names := []string{}
	if b, err := f.Buildings.Get(ctx, sc, rp.BuildingID); err == nil {
		names = append(names, b.Name)
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if format == FormatPDF {
		stamp := rp.UpdatedAt
		if rp.ProcessedAt != nil {
			stamp = *rp.ProcessedAt
		}
		return reportpdf.Render(reportpdf.Document{Payload: p, CompanyName: company.Name, BuildingNames: names, Locale: locale, GeneratedAt: stamp})
	}
	return reportxlsx.Render(p, company.Name, names, locale)
}

// write stores one file temp-then-rename and returns its path.
func (f Files) write(companyID, reportID uuid.UUID, format string, data []byte) (string, error) {
	dir := f.dir(companyID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	final := filepath.Join(dir, reportID.String()+ext(format))
	tmp, err := os.CreateTemp(dir, ".render-*"+ext(format))
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0o640); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", fmt.Errorf("report: store %s: %w", format, err)
	}
	return final, nil
}
