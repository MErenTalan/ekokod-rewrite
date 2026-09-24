package auth_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

func TestCheckPolicy(t *testing.T) {
	h := testHasher()
	reused, err := h.Hash("Gecmis!2025ab")
	require.NoError(t, err)
	current, err := h.Hash("Simdiki!2026ab")
	require.NoError(t, err)

	cases := []struct {
		name string
		in   auth.PolicyInput
		want []string
	}{
		{"too short", auth.PolicyInput{Password: "Kisa!1a"}, []string{"password_too_short"}},
		{"complexity and common", auth.PolicyInput{Password: "abcdefghijK1"}, []string{"password_complexity", "password_common"}},
		{"repeat", auth.PolicyInput{Password: "Aaaaa!2345xy"}, []string{"password_repeat"}},
		{"common sequence", auth.PolicyInput{Password: "Qwer!9876zz"}, []string{"password_common"}},
		{"name fragment", auth.PolicyInput{Password: "Yılmaz!2026ab", Name: "Mehmet Yılmaz"}, []string{"password_personal"}},
		{"name fragment turkish case", auth.PolicyInput{Password: "YILMAZ!2026ab", Name: "Mehmet Yılmaz"}, []string{"password_personal"}},
		{"email fragment", auth.PolicyInput{Password: "Kaya#2026abc", Email: "ayse.kaya@x.com"}, []string{"password_personal"}},
		{"company fragment", auth.PolicyInput{Password: "Enerji!2026x", CompanyName: "Bcem Enerji"}, []string{"password_personal"}},
		{"history reuse", auth.PolicyInput{Password: "Gecmis!2025ab", History: []string{reused}}, []string{"password_reused"}},
		{"current reuse", auth.PolicyInput{Password: "Simdiki!2026ab", CurrentHash: current}, []string{"password_reused"}},
		{"valid", auth.PolicyInput{Password: "Guvenli!Sifre-42", Name: "Ali Veli", Email: "ali@x.com", CompanyName: "Bcem Enerji", History: []string{reused}, CurrentHash: current}, nil},
		{"space counts as special", auth.PolicyInput{Password: "Guvenli Sifre42"}, nil},
		{"short fragments ignored", auth.PolicyInput{Password: "Guvenli!Sifre-42", Name: "Al Ve"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, auth.CheckPolicy(h, tc.in))
		})
	}

	long := auth.CheckPolicy(h, auth.PolicyInput{Password: "Aa1!" + strings.Repeat("xy", 63)})
	require.Contains(t, long, "password_too_long")
}
