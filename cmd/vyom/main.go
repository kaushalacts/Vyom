// Vyom - CDN media observability and optimization platform.
//
//	vyom [command]
//
// Available Commands:
//
//	completion  Generate shell completion scripts
//	config      Manage configuration
//	doctor      Check provider connectivity and configuration
//	init        Initialize a new Vyom project
//	migrate     Run database migrations
//	optimize    Run optimization (dry-run by default)
//	policy      Manage optimization policies
//	provider    Manage provider configurations
//	report      Generate reports from scan results
//	scan        Run a scan (fetch traffic, inventory, correlate, analyze)
//	serve       Start the REST API server
//	version     Print version information
package main

import (
	"fmt"
	"os"

	"github.com/kaushalacts/vyom/cmd/vyom/cli"
	"github.com/kaushalacts/vyom/internal/config"
	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository/sqlite"
	"github.com/kaushalacts/vyom/internal/version"
)

func main() {
	// Version is printed before any config is loaded, so it works even with
	// a broken config file.
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version.Get().String())
		os.Exit(0)
	}

	// Bootstrap a minimal logger for early messages (before config loads).
	earlyLog := logging.New(logging.Config{
		Level:   logging.LevelWarn,
		Format:  logging.FormatText,
		Service: "vyom",
	})

	// Load configuration.
	cfgPath := ""
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
			break
		}
		if arg == "-c" && i+1 < len(os.Args) {
			cfgPath = os.Args[i+1]
			break
		}
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		earlyLog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	// Reconfigure logger with loaded config.
	log := logging.New(cfg.Logging)
	log.Info("starting vyom", "version", version.Get().Version, "config", cfg.SourcePath)

	// Open database.
	db, err := sqlite.New(cfg.Database.Path)
	if err != nil {
		log.Error("database open failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// Run CLI.
	root := cli.NewRootCmd(log, db, cfg)
	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		os.Exit(1)
	}
}
