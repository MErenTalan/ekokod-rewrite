// Package tariff holds the pure tariff rules: validation (R128), resolution by
// date (02 §4), dated billing parameters (R106) and PTF+YEKDEM pricing (02 §7,
// R108, R109, R119, R124). It does no I/O; instants, the location and market
// data are arguments.
package tariff

// DivisionScale is the scale of every division in the billing domain (M-3).
const DivisionScale int32 = 20
