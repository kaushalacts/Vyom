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

func newOptimizeCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Run optimization (dry-run by default)",
		Long: `Optimize media assets according to a policy.

DRY RUN IS THE DEFAULT. No production objects are modified unless --apply
is passed with explicit confirmation.

Steps:
1. Load scan results and policy
2. Select assets matching policy criteria
3. For each asset: download, analyze, encode candidate
4. Validate candidate against quality gates (SSIM, PSNR)
5. Report projected savings
6. If --apply: write optimized copies (never overwrites originals)

Safety:
- Originals are never deleted or overwritten
- Optimized copies are written alongside originals
- Every operation is recorded in a manifest for rollback
- --dry-run is the default`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			scanID, _ := cmd.Flags().GetString("scan")
			policyName, _ := cmd.Flags().GetString("policy")
			assetFilter, _ := cmd.Flags().GetString("asset")
			apply, _ := cmd.Flags().GetBool("apply")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			if apply && dryRun {
				return providererr.Configuration("cli", "optimize", "--apply and --dry-run are mutually exclusive")
			}
			if apply {
				dryRun = false
			} else {
				dryRun = true
			}

			fmt.Fprintln(os.Stderr, "VYOM OPTIMIZE")
			fmt.Fprintln(os.Stderr, "=============")

			if dryRun {
				fmt.Fprintln(os.Stderr, "Mode: DRY RUN (no files will be modified)")
			} else {
				fmt.Fprintln(os.Stderr, "Mode: APPLY (optimized copies will be written)")
				if !confirm("This will write optimized copies to storage. Continue?") {
					fmt.Fprintln(os.Stderr, "Aborted.")
					return nil
				}
			}

			// Milestone 1: optimizer not implemented
			fmt.Fprintln(os.Stderr, "\nImage optimization (libvips) not yet implemented in Milestone 1.")
			fmt.Fprintln(os.Stderr, "This will be functional in Milestone 7+.")

			return nil
		},
	}
	cmd.Flags().String("scan", "", "scan ID (default: latest)")
	cmd.Flags().String("policy", "", "policy name (default: from config)")
	cmd.Flags().String("asset", "", "specific asset storage key to optimize")
	cmd.Flags().Bool("apply", false, "apply optimizations (requires confirmation)")
	cmd.Flags().Bool("dry-run", true, "simulate only (default)")
	return cmd
}
