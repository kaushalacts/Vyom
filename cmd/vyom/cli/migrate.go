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

func newMigrateCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		Long:  `Apply any pending database migrations. This is run automatically on startup.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := db.Migrate(cmd.Context()); err != nil {
				return providererr.Wrap(err, "cli", "migrate", providererr.KindInternal)
			}
			fmt.Fprintln(os.Stderr, "Migrations applied successfully.")
			return nil
		},
	}
	return cmd
}
