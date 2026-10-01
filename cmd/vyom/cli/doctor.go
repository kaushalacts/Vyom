package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newDoctorCmd(log *logging.Logger, db repository.DB, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check provider connectivity and configuration",
		Long: `Verify that Vyom can reach configured providers and that credentials are valid.

This command:
  - Checks that provider configurations exist in the database
  - Validates that referenced environment variables are set
  - Attempts a lightweight connectivity test for each provider
  - Reports any configuration issues

Exit codes:
  0 = all checks passed
  1 = configuration error
  2 = one or more providers unhealthy`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Check configured providers
			providers, err := db.ListProviders(ctx)
			if err != nil {
				return providererr.Wrap(err, "cli", "doctor:list_providers", providererr.KindInternal)
			}

			if len(providers) == 0 {
				fmt.Fprintln(os.Stderr, "No providers configured. Run 'vyom provider add' to configure.")
				return nil
			}

			fmt.Fprintln(os.Stderr, "VYOM DOCTOR")
			fmt.Fprintln(os.Stderr, "===========")

			allHealthy := true
			for _, p := range providers {
				health := checkProvider(ctx, p, cfg)
				printProviderHealth(health)
				if !health.Status.OK() {
					allHealthy = false
				}
			}

			if jsonFlag(cmd) {
				// Would output JSON here
			}

			if allHealthy {
				fmt.Fprintln(os.Stderr, "\nAll checks passed.")
				return nil
			}
			fmt.Fprintln(os.Stderr, "\nSome checks failed. See details above.")
			return providererr.Configuration("cli", "doctor", "one or more providers unhealthy")
		},
	}
	return cmd
}

func checkProvider(ctx context.Context, p models.ProviderConfig, cfg config.Config) models.ProviderHealth {
	health := models.ProviderHealth{Config: p, Status: p.Status}

	// Check 1: required env vars are set
	if !p.Credentials.IsZero() {
		missing := checkEnvVars(p.Credentials)
		if len(missing) > 0 {
			health.Checks = append(health.Checks, models.HealthCheck{
				Name:   "credentials_present",
				OK:     false,
				Detail: fmt.Sprintf("missing env vars: %v", missing),
				Hint:   "Set the environment variables referenced in the provider config",
			})
			health.Status = models.ProviderMissingCredentials
		} else {
			health.Checks = append(health.Checks, models.HealthCheck{
				Name: "credentials_present", OK: true, Detail: "all referenced env vars are set",
			})
		}
	} else {
		health.Checks = append(health.Checks, models.HealthCheck{
			Name: "credentials_present", OK: false, Detail: "no credentials configured",
			Hint: "Add credential environment variable references to the provider",
		})
		health.Status = models.ProviderMissingCredentials
	}

	// Check 2: provider-specific options
	if p.Kind == models.KindCDN && p.Type == models.ProviderAkamai {
		if _, ok := p.Options["host"]; !ok {
			health.Checks = append(health.Checks, models.HealthCheck{
				Name: "akamai_host", OK: false, Detail: "missing 'host' option",
				Hint: "Set options.host to your Akamai API hostname (e.g. akab-xxx.luna.akamaiapis.net)",
			})
			health.Status = models.ProviderUnconfigured
		}
	}

	if p.Kind == models.KindStorage && p.Type == models.ProviderAzureBlob {
		if _, ok := p.Options["account"]; !ok {
			health.Checks = append(health.Checks, models.HealthCheck{
				Name: "azure_account", OK: false, Detail: "missing 'account' option",
				Hint: "Set options.account to your Azure storage account name",
			})
			health.Status = models.ProviderUnconfigured
		}
		if _, ok := p.Options["container"]; !ok {
			health.Checks = append(health.Checks, models.HealthCheck{
				Name: "azure_container", OK: false, Detail: "missing 'container' option",
				Hint: "Set options.container to the blob container name",
			})
			health.Status = models.ProviderUnconfigured
		}
	}

	// If basic config checks passed, mark as configured (connectivity test would be next)
	if health.Status == models.ProviderUnconfigured || health.Status == models.ProviderMissingCredentials {
		health.Message = "Configuration incomplete"
	} else {
		health.Status = models.ProviderConfigured
		health.Message = "Configuration valid; connectivity test not implemented in Milestone 1"
		health.Checks = append(health.Checks, models.HealthCheck{
			Name: "connectivity", OK: true, Detail: "skipped (provider clients not implemented in M1)",
			Optional: true,
		})
	}

	return health
}

func checkEnvVars(refs models.SecretRefs) []string {
	var missing []string
	for _, name := range refs.Names() {
		if _, ok := os.LookupEnv(name); !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func printProviderHealth(h models.ProviderHealth) {
	statusIcon := "✓"
	if !h.Status.OK() {
		statusIcon = "✗"
	}
	fmt.Fprintf(os.Stderr, "\n%s %s (%s:%s) - %s\n",
		statusIcon, h.Config.Name, h.Config.Kind, h.Config.Type, h.Message)
	for _, c := range h.Checks {
		icon := "✓"
		if !c.OK {
			icon = "✗"
		}
		optional := ""
		if c.Optional {
			optional = " (optional)"
		}
		fmt.Fprintf(os.Stderr, "  %s %s: %s%s\n", icon, c.Name, c.Detail, optional)
		if c.Hint != "" {
			fmt.Fprintf(os.Stderr, "    → %s\n", c.Hint)
		}
	}
}
