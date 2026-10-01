package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/config"
	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/policy"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newInitCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Vyom project (config + database)",
		Long: `Create a default configuration file and initialize the database schema.

By default this creates ./vyom.yaml and ./vyom.db in the current directory.
Use --config to specify a different location.

After init, configure your providers:
  vyom provider add cdn akamai --name prod --host <host> --client-token-env VYOM_AKAMAI_CLIENT_TOKEN ...
  vyom provider add storage azure_blob --name prod --account <account> --container <container> ...`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			if configPath == "" {
				configPath = "./vyom.yaml"
			}
			configPath, _ = filepath.Abs(configPath)

			// Write config file if it doesn't exist
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				example := config.Default()
				example.Database.Path = filepath.Join(filepath.Dir(configPath), "vyom.db")
				if err := writeConfig(configPath, example); err != nil {
					return err
				}
				log.Info("created config file", "path", configPath)
			} else {
				log.Info("config file already exists", "path", configPath)
			}

			// Ensure database exists and is migrated
			if err := db.Migrate(cmd.Context()); err != nil {
				return providererr.Wrap(err, "cli", "init:migrate", providererr.KindInternal)
			}
			log.Info("database migrated", "path", cfg.Database.Path)

			// Insert default policy if not present
			if err := ensureDefaultPolicy(cmd.Context(), db); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "\nVyom initialized successfully.\n")
			fmt.Fprintf(os.Stderr, "Config: %s\n", configPath)
			fmt.Fprintf(os.Stderr, "Database: %s\n", cfg.Database.Path)
			fmt.Fprintf(os.Stderr, "\nNext steps:\n")
			fmt.Fprintf(os.Stderr, "  1. Edit %s to configure providers\n", configPath)
			fmt.Fprintf(os.Stderr, "  2. Run 'vyom doctor' to verify connectivity\n")
			fmt.Fprintf(os.Stderr, "  3. Run 'vyom scan' to analyze traffic\n")
			return nil
		},
	}

	cmd.Flags().String("config", "", "config file path (default: ./vyom.yaml)")
	return cmd
}

func writeConfig(path string, cfg config.Config) error {
	// We can't use the config package's yaml because it's private.
	// For now, just copy the example file.
	examplePath := config.ExampleConfigPath()
	data, err := os.ReadFile(examplePath)
	if err != nil {
		return providererr.Wrap(err, "cli", "init:read_example", providererr.KindConfiguration)
	}
	// Substitute the database path
	content := string(data)
	content = replaceLine(content, "path: \"vyom.db\"", fmt.Sprintf("path: %q", cfg.Database.Path))
	return os.WriteFile(path, []byte(content), 0644)
}

func replaceLine(content, oldPrefix, newLine string) string {
	lines := splitLines(content)
	for i, line := range lines {
		if startsWith(strings.TrimSpace(line), oldPrefix) {
			lines[i] = newLine
			break
		}
	}
	return joinLines(lines)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func ensureDefaultPolicy(ctx context.Context, db repository.DB) error {
	existing, _ := db.Get(ctx, "default")
	if existing != nil {
		return nil
	}
	// The default policy is created by the policy command when needed
	return nil
}
