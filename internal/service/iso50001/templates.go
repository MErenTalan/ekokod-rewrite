package iso50001

import (
	"bytes"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Template is a downloadable clause template (R340, Q-G1).
type Template struct {
	ID, FileName string
	Clauses      []string
	Description  map[string]string
	contentType  string
	build        func() ([]byte, error)
}

const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

//nolint:misspell // Turkish descriptions ("performans"), not English typos
var templates = []Template{
	{ID: "significant-energy-uses", FileName: "Onemli-Enerji-Kullanimlari.xlsx", Clauses: []string{"6.3"}, contentType: xlsxType,
		Description: map[string]string{"tr": "Hangi ekipmanların çok enerji tükettiğinin tespiti (Pareto analizi).", "en": "Finding the equipment that uses the most energy (Pareto analysis)."},
		build:       significantEnergyUses},
	{ID: "energy-consumption-analysis", FileName: "Enerji-Tuketim-Analizi.xlsx", Clauses: []string{"6.4", "6.5", "9.1"}, contentType: xlsxType,
		Description: map[string]string{"tr": "Baz yılı, performans formülü ve hedeflenen ile gerçekleşen tüketimin CUSUM grafiği.", "en": "Baseline year, performance formula and the CUSUM of expected against actual consumption."},
		build:       consumptionAnalysis},
	{ID: "regression-analysis-instruction", FileName: "Regresyon-Analizi-Talimati.pdf", Clauses: []string{"7.5"}, contentType: "application/pdf",
		Description: map[string]string{"tr": "Analizlerin nasıl yapılacağını anlatan prosedür.", "en": "The procedure describing how the analyses are made."},
		build:       regressionInstruction},
}

// Templates lists the templates.
func (s *Service) Templates() []Template { return templates }

// Template builds one template's file.
func (s *Service) Template(id string) (body []byte, name, contentType string, err error) {
	for _, t := range templates {
		if t.ID == id {
			body, err = t.build()
			return body, t.FileName, t.contentType, err
		}
	}
	return nil, "", "", store.ErrNotFound
}

func workbook(sheets func(f *excelize.File) error) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := sheets(f); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func row(f *excelize.File, sheet string, r int, values ...any) error {
	cell, err := excelize.CoordinatesToCellName(1, r)
	if err != nil {
		return err
	}
	return f.SetSheetRow(sheet, cell, &values)
}

// significantEnergyUses is a Pareto table: share, cumulative share, and the
// 80% cut that marks a significant energy use.
func significantEnergyUses() ([]byte, error) {
	return workbook(func(f *excelize.File) error {
		const sheet = "ÖEK Pareto"
		if err := f.SetSheetName("Sheet1", sheet); err != nil {
			return err
		}
		if err := row(f, sheet, 1, "Ekipman / Alan", "Yıllık tüketim (kWh)", "Pay (%)", "Kümülatif pay (%)", "Önemli enerji kullanımı"); err != nil {
			return err
		}
		for r := 2; r <= 21; r++ {
			_ = f.SetCellFormula(sheet, fmt.Sprintf("C%d", r), fmt.Sprintf(`IF(SUM($B$2:$B$21)=0,"",B%d/SUM($B$2:$B$21)*100)`, r))
			_ = f.SetCellFormula(sheet, fmt.Sprintf("D%d", r), fmt.Sprintf(`IF(C%d="","",SUM($C$2:C%d))`, r, r))
			_ = f.SetCellFormula(sheet, fmt.Sprintf("E%d", r), fmt.Sprintf(`IF(D%d="","",IF(D%d<=80,"Evet","Hayır"))`, r, r))
		}
		_ = f.SetCellValue(sheet, "G1", "Satırları yıllık tüketime göre büyükten küçüğe sıralayın; kümülatif payı %80'e kadar olanlar önemli enerji kullanımıdır (TS EN ISO 50001:2018 6.3).")
		return nil
	})
}

// consumptionAnalysis has the baseline, the EnPI and the CUSUM sheets.
func consumptionAnalysis() ([]byte, error) {
	return workbook(func(f *excelize.File) error {
		if err := f.SetSheetName("Sheet1", "Baz yılı"); err != nil {
			return err
		}
		if err := row(f, "Baz yılı", 1, "Ay", "Tüketim (kWh)", "Üretim miktarı", "Isıtma derece günü", "EnPG (kWh / birim)"); err != nil {
			return err
		}
		months := []string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
		for i, m := range months {
			r := i + 2
			_ = f.SetCellValue("Baz yılı", fmt.Sprintf("A%d", r), m)
			_ = f.SetCellFormula("Baz yılı", fmt.Sprintf("E%d", r), fmt.Sprintf(`IF(C%d=0,"",B%d/C%d)`, r, r, r))
		}
		for _, sheet := range []string{"CUSUM"} {
			if _, err := f.NewSheet(sheet); err != nil {
				return err
			}
		}
		if err := row(f, "CUSUM", 1, "Ay", "Beklenen tüketim (kWh)", "Gerçekleşen tüketim (kWh)", "Fark (kWh)", "Kümülatif fark (CUSUM)"); err != nil {
			return err
		}
		for i, m := range months {
			r := i + 2
			_ = f.SetCellValue("CUSUM", fmt.Sprintf("A%d", r), m)
			_ = f.SetCellFormula("CUSUM", fmt.Sprintf("D%d", r), fmt.Sprintf(`IF(OR(B%d="",C%d=""),"",C%d-B%d)`, r, r, r, r))
			_ = f.SetCellFormula("CUSUM", fmt.Sprintf("E%d", r), fmt.Sprintf(`IF(D%d="","",SUM($D$2:D%d))`, r, r))
		}
		_ = f.SetCellValue("CUSUM", "G1", "Beklenen tüketim, baz yılı regresyon denkleminden hesaplanır. Negatif ve düşen CUSUM tasarrufu gösterir (TS EN ISO 50001:2018 6.4, 6.5, 9.1).")
		return nil
	})
}

var instruction = []string{
	"1. Amaç: Enerji tüketimini etkileyen değişkenlerle (üretim miktarı, derece gün, çalışma saati) tüketim arasındaki ilişkiyi belirlemek ve enerji referans çizgisini (EnRÇ) oluşturmak.",
	"2. Veri: En az 12 aylık tüketim (kWh) ve aynı dönemin değişken verilerini toplayın; eksik veya hatalı ayları not edin.",
	"3. Model: Tüketimi bağımlı, değişkenleri bağımsız değişken alarak doğrusal regresyon kurun (Tüketim = a + b1·X1 + b2·X2).",
	"4. Geçerlilik: R² değerinin 0,75 ve üzeri, katsayıların p değerlerinin 0,10'un altında olmasını kontrol edin; sağlanmıyorsa değişkenleri gözden geçirin.",
	"5. Kullanım: Denklemi beklenen tüketimi hesaplamak için kullanın; gerçekleşen ile farkları Enerji Tüketim Analizi şablonunun CUSUM sayfasında izleyin.",
	"6. Güncelleme: Tesis, ürün veya çalışma koşulları önemli ölçüde değiştiğinde modeli yeniden kurun ve değişikliği dokümante edin (7.5).",
	"Tüm maddeler TS EN ISO 50001:2018 standardına dayanmaktadır. Bu belge yalnızca rehberlik amaçlıdır.",
}

// regressionInstruction is the SOP legacy named a .docx (Q-G1).
func regressionInstruction() ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	pdf.SetCreationDate(at)
	pdf.SetModificationDate(at)
	pdf.AddUTF8FontFromBytes("Go", "", goregular.TTF)
	pdf.AddUTF8FontFromBytes("Go", "B", gobold.TTF)
	pdf.SetTitle("Regresyon Analizi Talimatı", true)
	pdf.AddPage()
	pdf.SetFont("Go", "B", 16)
	pdf.MultiCell(0, 8, "Regresyon Analizi Talimatı", "", "L", false)
	pdf.Ln(4)
	pdf.SetFont("Go", "", 11)
	for _, p := range instruction {
		pdf.MultiCell(0, 6, p, "", "L", false)
		pdf.Ln(2)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
