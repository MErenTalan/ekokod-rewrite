// Package icmal parses supplier billing summaries (icmal) and derives KBK
// coefficients, prices and named tax rates from them (02 §8 as amended by
// R129–R133). It is pure: reading the file into a Table is the caller's job.
package icmal
