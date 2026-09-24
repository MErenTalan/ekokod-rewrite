package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// newUserPasswordEnv carries the new user's password; never a flag, never a default (R182).
const newUserPasswordEnv = "EKOKOD_NEW_USER_PASSWORD"

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "user", Short: "Manage users"}
	var email, name, role, companyID, companyName string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a user; the password is read from " + newUserPasswordEnv,
		RunE: func(cmd *cobra.Command, _ []string) error {
			password := os.Getenv(newUserPasswordEnv)
			if password == "" {
				return fmt.Errorf("%s is required", newUserPasswordEnv)
			}
			if (companyID == "") == (companyName == "") {
				return errors.New("exactly one of --company-id and --company-name is required")
			}
			in := seed.UserInput{Email: email, Name: name, Role: model.UserRole(role), CompanyName: companyName, Password: password}
			if companyID != "" {
				id, err := uuid.Parse(companyID)
				if err != nil {
					return fmt.Errorf("--company-id: %w", err)
				}
				in.CompanyID = id
			}
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			pool, err := postgres.NewPool(ctx, cfg.DB, newCommandLogger(cfg, os.Stderr))
			if err != nil {
				return err
			}
			defer pool.Close()
			user, err := seed.CreateUser(ctx, pool, auth.Hasher{Pepper: cfg.Security.PasswordPepper, Cost: cfg.Security.BcryptCost}, in, time.Now())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "created user %s (%s) in company %s\n", user.Email, user.Role, user.CompanyID)
			return err
		},
	}
	create.Flags().StringVar(&email, "email", "", "e-mail (required)")
	create.Flags().StringVar(&name, "name", "", "full name (required)")
	create.Flags().StringVar(&role, "role", "", "admin|company_admin|company_readonly_admin|building_admin|building_readonly_admin")
	create.Flags().StringVar(&companyID, "company-id", "", "existing company id")
	create.Flags().StringVar(&companyName, "company-name", "", "company name; created when missing")
	for _, f := range []string{"email", "name", "role"} {
		_ = create.MarkFlagRequired(f)
	}
	cmd.AddCommand(create)
	return cmd
}
