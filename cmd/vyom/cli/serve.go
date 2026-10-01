package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/api"
	"github.com/kaushalacts/vyom/internal/config"
	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newServeCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the REST API server",
		Long: `Start the Vyom REST API server.

The server exposes:
- /api/v1/health        health check
- /api/v1/version       version info
- /api/v1/assets        asset listings
- /api/v1/impacts       impact rankings
- /api/v1/scans         scan job history
- /api/v1/optimizations optimization jobs
- /api/v1/policies      policy management
- /api/v1/providers     provider management`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			router := chi.NewRouter()
			api.RegisterRoutes(router, log, db, cfg)

			srv := &http.Server{
				Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
				Handler:      router,
				ReadTimeout:  cfg.Server.ReadTimeout,
				WriteTimeout: cfg.Server.WriteTimeout,
				IdleTimeout:  cfg.Server.IdleTimeout,
			}

			// Graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				log.Info("shutting down server")
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					log.Error("server shutdown error", "error", err)
				}
			}()

			log.Info("starting server", "addr", srv.Addr)
			fmt.Fprintf(os.Stderr, "Vyom API server listening on %s\n", srv.Addr)

			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				return providererr.Wrap(err, "cli", "serve", providererr.KindInternal)
			}
			log.Info("server stopped")
			return nil
		},
	}
	return cmd
}
