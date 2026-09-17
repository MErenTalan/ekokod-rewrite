//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func cliEnv(t *testing.T, dsn string) {
	t.Helper()
	for k, v := range map[string]string{
		"EKOKOD_ENV": "development", "EKOKOD_PUBLIC_URL": "http://localhost:3000", "EKOKOD_DB_URL": dsn,
		"EKOKOD_REDIS_URL": "redis://localhost:6379/0", "EKOKOD_ENCRYPTION_KEY": "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"EKOKOD_JWT_SIGNING_KEY": "jwt-signing-key-at-least-32-chars-long!!", "EKOKOD_PASSWORD_PEPPER": "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!", "EKOKOD_STORAGE_ROOT": "/tmp/ekokod",
		"EKOKOD_EPIAS_USERNAME": "u", "EKOKOD_EPIAS_PASSWORD": "p", "EKOKOD_ML_API_KEY": "k",
	} {
		t.Setenv(k, v)
	}
}

func TestUserCreateCLI(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	cliEnv(t, pool.Config().ConnString())
	ctx := context.Background()
	args := []string{"user", "create", "--email", "kurucu@ekokod.test", "--name", "Kurucu Yönetici", "--role", "admin", "--company-name", "Ekokod Platform"}

	t.Setenv("EKOKOD_NEW_USER_PASSWORD", "")
	require.ErrorContains(t, cli.Execute(ctx, args, &bytes.Buffer{}), "EKOKOD_NEW_USER_PASSWORD is required")

	t.Setenv("EKOKOD_NEW_USER_PASSWORD", "kisa")
	require.ErrorContains(t, cli.Execute(ctx, args, &bytes.Buffer{}), "password_too_short")

	t.Setenv("EKOKOD_NEW_USER_PASSWORD", "Guvenli!Sifre-42")
	var out bytes.Buffer
	require.NoError(t, cli.Execute(ctx, args, &out))
	require.Contains(t, out.String(), "created user kurucu@ekokod.test (admin)")

	user, err := admin.NewAuthRepository(pool).UserByEmail(ctx, "kurucu@ekokod.test")
	require.NoError(t, err)
	ok, _ := auth.Hasher{Pepper: []byte("password-pepper-at-least-32-chars-long!!"), Cost: 12}.Verify(user.PasswordHash, "Guvenli!Sifre-42")
	require.True(t, ok)

	demo := []string{"user", "create", "--email", "d@ekokod.test", "--name", "Demo", "--role", "demo", "--company-id", user.CompanyID.String()}
	require.ErrorContains(t, cli.Execute(ctx, demo, &bytes.Buffer{}), "seed demo")

	again := []string{"user", "create", "--email", "ikinci@ekokod.test", "--name", "İkinci", "--role", "company_admin", "--company-name", "ekokod platform"}
	require.NoError(t, cli.Execute(ctx, again, &bytes.Buffer{}))
	second, err := admin.NewAuthRepository(pool).UserByEmail(ctx, "ikinci@ekokod.test")
	require.NoError(t, err)
	require.Equal(t, user.CompanyID, second.CompanyID, "an existing company is matched case-insensitively")
}
