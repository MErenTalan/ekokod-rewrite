// Package billing generates, supersedes and reads invoices (05 §7, R105–R135)
// around the pure internal/domain/billing engine. Consumption is always the
// invoice-grade Billing path, never analytics (R61). Import it as billingsvc.
package billing
