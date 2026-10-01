package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/config"
	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/version"
)

// RootCmd builds the complete CLI command tree.
func NewRootCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "vyom",
		Short: "Vyom - CDN media observability and optimization",
		Long: `Vyom finds the media assets consuming your CDN bandwidth.

Vyom correlates CDN traffic with object storage, ranks assets by bandwidth
impact, and projects optimization savings - all without modifying production
objects unless explicitly requested.

Quick start:
  vyom init                # create config and database
  vyom doctor              # check configuration
  vyom scan                # analyze CDN traffic (needs providers configured)
  vyom report              # show optimization opportunities
  vyom optimize --dry-run  # simulate optimization`,
		Version: version.Get().Version,
		// Disable Cobra's default completion command; we provide our own.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Ensure DB is migrated before any command runs.
			return db.Migrate(cmd.Context())
		},
	}

	// Global flags
	root.PersistentFlags().String("config", "", "config file path (default: ./vyom.yaml)")
	root.PersistentFlags().Bool("json", false, "output as JSON")
	_ = root.PersistentFlags().MarkHidden("config") // handled in main.go

	// Add subcommands
	root.AddCommand(newVersionCmd())
	root.AddCommand(newInitCmd(log, db, cfg))
	root.AddCommand(newDoctorCmd(log, db, cfg))
	root.AddCommand(newMigrateCmd(log, db))
	root.AddCommand(newConfigCmd(log, cfg))
	root.AddCommand(newProviderCmd(log, db))
	root.AddCommand(newPolicyCmd(log, db))
	root.AddCommand(newScanCmd(log, db, cfg))
	root.AddCommand(newReportCmd(log, db, cfg))
	root.AddCommand(newOptimizeCmd(log, db, cfg))
	root.AddCommand(newServeCmd(log, db, cfg))
	root.AddCommand(newCompletionCmd())

	return root
}

// jsonFlag returns whether --json was passed.
func jsonFlag(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// printJSON marshals v to JSON and writes to stdout.
func printJSON(v any) error {
	enc := jsonEncoder()
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printTable prints a simple text table for the given rows.
//
// rows is a slice of maps or structs. For maps, keys are column headers.
// For structs, exported field names are headers.
func printTable(rows any) error {
	// Simple implementation: just JSON for now, pretty table in Milestone 10.
	return printJSON(rows)
}

// confirm prompts the user for yes/no confirmation.
func confirm(prompt string) bool {
	fmt.Fprint(os.Stderr, prompt+" [y/N]: ")
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}

// jsonEncoder returns a JSON encoder that writes to stdout.
func jsonEncoder() *jsonEncoderType {
	return &jsonEncoderType{enc: nil}
}

type jsonEncoderType struct {
	enc any
}

func (j *jsonEncoderType) Encode(v any) error {
	return nil // placeholder
}
