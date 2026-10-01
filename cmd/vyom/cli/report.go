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

func newReportCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Generate reports from scan results",
		Long: `Generate reports from completed scans.

Reports show:
- Top bandwidth contributors
- Optimization opportunities
- Projected savings
- Cost estimates (if pricing configured)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			scanID, _ := cmd.Flags().GetString("scan")
			format, _ := cmd.Flags().GetString("format")
			limit, _ := cmd.Flags().GetInt("limit")

			fmt.Fprintln(os.Stderr, "VYOM REPORT")
			fmt.Fprintln(os.Stderr, "===========")

			if scanID == "" {
				// Use latest scan
				latest, err := db.LatestScan(ctx)
				if err != nil {
					return providererr.Wrap(err, "cli", "report:latest", providererr.KindInternal)
				}
				if latest == nil {
					fmt.Fprintln(os.Stderr, "No scans found. Run 'vyom scan' first.")
					return nil
				}
				scanID = latest.ID
			}

			// Milestone 1: report generation not implemented
			fmt.Fprintln(os.Stderr, "\nReport generation not yet implemented in Milestone 1.")
			fmt.Fprintln(os.Stderr, "This will be functional in Milestone 6+.")

			return nil
		},
	}
	cmd.Flags().String("scan", "", "scan ID (default: latest)")
	cmd.Flags().String("format", "text", "output format (text, json, csv)")
	cmd.Flags().Int("limit", 20, "number of top assets to show")
	return cmd
}
