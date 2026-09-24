package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// newMigrateLegacyCmd is 08 §11's legacy toolkit (F14a Q-J1): inventory, extract, transform.
func newMigrateLegacyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "legacy", Short: "Move the legacy system's data in: inventory, extract, transform, load (08-migration.md)"}
	cmd.AddCommand(newLegacyInventoryCmd(), newLegacyExtractCmd(), newLegacyTransformCmd(), newLegacyLoadCmd())
	return cmd
}

type mongoFlags struct{ uri, db string }

func (m *mongoFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&m.uri, "uri", os.Getenv("EKOKOD_LEGACY_MONGODB_URI"), "legacy MongoDB URI (a read-only user; default $EKOKOD_LEGACY_MONGODB_URI)")
	cmd.Flags().StringVar(&m.db, "db", os.Getenv("EKOKOD_LEGACY_MONGODB_DB"), "legacy database name (default $EKOKOD_LEGACY_MONGODB_DB)")
}

func (m *mongoFlags) open(ctx context.Context) (*legacy.MongoSource, error) {
	if m.uri == "" || m.db == "" {
		return nil, errors.New("--uri and --db are required")
	}
	return legacy.OpenMongo(ctx, m.uri, m.db)
}

func newLegacyInventoryCmd() *cobra.Command {
	var mf mongoFlags
	var fromExtract, artifacts, jsonOut string
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Read-only survey of the legacy database and artifacts (08 §3)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			var src legacy.Source
			if fromExtract != "" {
				es, err := legacy.OpenExtract(fromExtract)
				if err != nil {
					return err
				}
				src = es
			} else {
				ms, err := mf.open(ctx)
				if err != nil {
					return err
				}
				defer func() { _ = ms.Close(context.WithoutCancel(ctx)) }()
				src = ms
			}
			var dirs []string
			if artifacts != "" {
				dirs = strings.Split(artifacts, ",")
			}
			inv, err := legacy.TakeInventory(ctx, src, time.Now(), dirs)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), inv.Text()); err != nil {
				return err
			}
			if jsonOut == "" {
				return nil
			}
			b, err := json.MarshalIndent(inv, "", "  ")
			if err != nil {
				return err
			}
			return os.WriteFile(jsonOut, append(b, '\n'), 0o600)
		},
	}
	mf.bind(cmd)
	cmd.Flags().StringVar(&fromExtract, "from-extract", "", "survey an extract directory instead of the live database")
	cmd.Flags().StringVar(&artifacts, "artifacts", "", "comma-separated artifact directories, e.g. /opt/bills,/opt/reports,/opt/documents")
	cmd.Flags().StringVar(&jsonOut, "json", "", "also write the inventory as JSON to this file")
	return cmd
}

func newLegacyExtractCmd() *cobra.Command {
	var mf mongoFlags
	var out string
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Legacy MongoDB → checksummed NDJSON, read-only and re-runnable (08 §4)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				return errors.New("--out is required")
			}
			ms, err := mf.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = ms.Close(context.WithoutCancel(cmd.Context())) }()
			m, err := legacy.Extract(cmd.Context(), ms, out)
			if err != nil {
				return err
			}
			for _, e := range m.Collections {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %d documents, sha256 %s\n", e.Collection, e.Count, e.SHA256); err != nil {
					return err
				}
			}
			return nil
		},
	}
	mf.bind(cmd)
	cmd.Flags().StringVar(&out, "out", "", "staging directory for the extract")
	return cmd
}

func newLegacyTransformCmd() *cobra.Command {
	var extract, out, confirmations, now string
	cmd := &cobra.Command{
		Use:   "transform",
		Short: "Normalise and validate the extract into COPY-ready files with rejections (08 §5)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extract == "" || out == "" {
				return errors.New("--extract and --out are required")
			}
			cfg, err := config.FromEnv()
			if err != nil {
				return err
			}
			cipher, err := crypto.NewCipher(cfg.Security.EncryptionKey)
			if err != nil {
				return err
			}
			opt := legacy.TransformOptions{Cipher: cipher, Now: time.Now().UTC(),
				Keys: legacy.Keys{Primary: string(cfg.Security.LegacyEncryptionKey), Fallback: os.Getenv("EKOKOD_LEGACY_ENCRYPTION_KEY_FALLBACK")}}
			if now != "" {
				if opt.Now, err = time.Parse(time.RFC3339, now); err != nil {
					return fmt.Errorf("--now: %w", err)
				}
			}
			if confirmations != "" {
				if opt.Confirmations, err = readConfirmations(confirmations); err != nil {
					return err
				}
			}
			res, err := legacy.Transform(extract, out, opt)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		},
	}
	cmd.Flags().StringVar(&extract, "extract", "", "extract directory (its manifest is verified first)")
	cmd.Flags().StringVar(&out, "out", "", "staging directory for the transformed files")
	cmd.Flags().StringVar(&confirmations, "confirmations", "", "manual_multipliers.csv with the answer column filled (raw|multiplied)")
	cmd.Flags().StringVar(&now, "now", "", "RFC 3339 instant bounding plausible readings; fix it to make reruns identical")
	return cmd
}

func newLegacyLoadCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "load",
		Short: "Write a transform directory into Postgres: ordered, per-stage transactional, re-runnable (08 §6)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dir == "" {
				return errors.New("--dir is required")
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
			if pending, err := postgres.PendingMigrations(ctx, cfg.DB.URL); err != nil {
				return err
			} else if pending > 0 {
				return fmt.Errorf("the schema has %d pending migrations: run `ekokod migrate up` first", pending)
			}
			rep, err := legacy.Load(ctx, admin.NewLegacyLoader(pool), dir)
			if err != nil {
				return err
			}
			for _, w := range rep.Warnings {
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w); err != nil {
					return err
				}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rep)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "a transform output directory")
	return cmd
}

// readConfirmations reads an answered manual_multipliers.csv (Q-J5).
func readConfirmations(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec // an operator-supplied file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i, r := range records {
		if i == 0 || len(r) < 7 || r[6] == "" {
			continue
		}
		answer := strings.ToLower(strings.TrimSpace(r[6]))
		if answer != "raw" && answer != "multiplied" {
			return nil, fmt.Errorf("%s line %d: answer must be raw or multiplied, got %q", path, i+1, r[6])
		}
		out[r[0]] = answer
	}
	return out, nil
}
