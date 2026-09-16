package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
)

// InvoiceFixture is a real invoice to reconcile against the engine. Total is
// vat_base + vat, never the supplier's rounded payable (I-2).
type InvoiceFixture struct {
	Description string        `json:"description"`
	Input       billing.Input `json:"input"`
	Expected    struct {
		Lines []struct {
			Code   string          `json:"code"`
			Amount decimal.Decimal `json:"amount"`
		} `json:"lines"`
		VatBase decimal.Decimal `json:"vat_base"`
		Vat     decimal.Decimal `json:"vat"`
		Total   decimal.Decimal `json:"total"`
	} `json:"expected"`
	Explanations map[string]string `json:"explanations"`
}

// ErrUnexplainedDifference is returned when a line differs without an explanation.
var ErrUnexplainedDifference = errors.New("compare-invoice: unexplained difference")

func newCompareInvoiceCmd() *cobra.Command {
	var fixture string
	cmd := &cobra.Command{
		Use:   "compare-invoice",
		Short: "Compare an engine-computed invoice with a real invoice fixture, line by line",
		RunE: func(cmd *cobra.Command, _ []string) error {
			raw, err := os.ReadFile(fixture)
			if err != nil {
				return err
			}
			return CompareInvoice(raw, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&fixture, "fixture", "", "fixture JSON file")
	_ = cmd.MarkFlagRequired("fixture")
	return cmd
}

// CompareInvoice prints code | expected | computed | diff | explanation and
// fails on any non-zero difference that has no explanation.
func CompareInvoice(raw []byte, out io.Writer) error {
	var f InvoiceFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		return fmt.Errorf("compare-invoice: fixture: %w", err)
	}
	inv, err := billing.Compute(f.Input)
	if err != nil {
		return fmt.Errorf("compare-invoice: compute: %w", err)
	}
	computed := map[string]decimal.Decimal{}
	for _, l := range inv.Lines {
		computed[l.Code] = computed[l.Code].Add(l.Amount)
	}
	type row struct {
		code               string
		expected, computed decimal.Decimal
	}
	var rows []row
	seen := map[string]bool{}
	for _, l := range f.Expected.Lines {
		rows = append(rows, row{l.Code, l.Amount, computed[l.Code]})
		seen[l.Code] = true
	}
	for _, l := range inv.Lines {
		if !seen[l.Code] {
			rows = append(rows, row{l.Code, decimal.Zero, computed[l.Code]})
			seen[l.Code] = true
		}
	}
	rows = append(rows, row{"vat_base", f.Expected.VatBase, inv.VatBase}, row{"vat_total", f.Expected.Vat, inv.VatCost},
		row{"total", f.Expected.Total, inv.VatBase.Add(inv.VatCost).Sub(inv.GenerationCredit)})

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t  %s\n", "code", "expected", "computed", "diff", "explanation")
	unexplained := 0
	for _, r := range rows {
		diff := r.computed.Sub(r.expected)
		note := f.Explanations[r.code]
		if !diff.IsZero() && note == "" {
			unexplained++
			note = "UNEXPLAINED"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t  %s\n", r.code, r.expected.StringFixed(2), r.computed.StringFixed(2), diff.StringFixed(2), note)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "\nComputed amounts derive from unrounded quantities and prices; re-deriving them from the stored 4/6-decimal columns can differ by a kuruş (M-8).")
	if unexplained > 0 {
		return fmt.Errorf("%w: %d line(s)", ErrUnexplainedDifference, unexplained)
	}
	return nil
}
