// Package report builds the monthly and yearly energy reports of 01 §7.14
// from per-building, per-month figures (02 §10). It is pure: no I/O, no
// clock. Every figure is nullable, because a report that prints 0 for a
// month nobody measured is lying (R258).
package report
