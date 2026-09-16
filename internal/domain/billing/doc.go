// Package billing computes invoices (02 §5, §6 as amended by R107–R135): the
// invoice period, net consumption, energy, distribution, power, reactive,
// extra charges, named taxes, VAT and total, plus building and company
// aggregation. It is pure; every rounding is R112's.
package billing
