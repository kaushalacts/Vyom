package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newScanCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Run a scan (fetch traffic, inventory, correlate, analyze)",
		Long: `Run a complete scan pipeline:

1. Validate configuration
2. Fetch CDN traffic for the configured window
3. Fetch storage object inventory
4. Correlate CDN URLs with storage objects
5. Detect media assets
6. Calculate impact scores
7. Persist results
8. Generate report

The scan window is determined by --from/--to flags, or by the config's
scan.days_back (default 30 days).

Providers must be configured with 'vyom provider add' before running a scan.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Get window from flags or config
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")

			fmt.Fprintln(os.Stderr, "VYOM SCAN")
			fmt.Fprintln(os.Stderr, "=========")

			// Check providers
			cdnProvider, _ := cmd.Flags().GetString("cdn")
			storageProvider, _ := cmd.Flags().GetString("storage")

			if cdnProvider == "" {
				fmt.Fprintln(os.Stderr, "Error: CDN provider not specified. Use --cdn or configure default in config.")
				return providererr.Configuration("cli", "scan", "CDN provider required")
			}
			if storageProvider == "" {
				fmt.Fprintln(os.Stderr, "Error: Storage provider not specified. Use --storage or configure default in config.")
				return providererr.Configuration("cli", "scan", "Storage provider required")
			}

			// Milestone 1: provider implementations don't exist yet
			fmt.Fprintln(os.Stderr, "\nProvider implementations (Akamai, Azure Blob) not yet implemented in Milestone 1.")
			fmt.Fprintln(os.Stderr, "This command will be functional in Milestone 4+.")
			fmt.Fprintln(os.Stderr, "\nFor now, you can:")
			fmt.Fprintln(os.Stderr, "  - Use 'vyom demo' to see a simulated scan")
			fmt.Fprintln(os.Stderr, "  - Use 'vyom scan --mock' to run against local fixtures (Milestone 2)")

			return nil
		},
	}
	cmd.Flags().String("from", "", "window start (YYYY-MM-DD)")
	cmd.Flags().String("to", "", "window end (YYYY-MM-DD)")
	cmd.Flags().String("cdn", "", "CDN provider ID")
	cmd.Flags().String("storage", "", "Storage provider ID")
	cmd.Flags().String("policy", "", "Optimization policy name")
	cmd.Flags().Bool("mock", false, "run against local fixtures (not implemented in M1)")
	return cmd
}
