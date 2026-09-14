// Package app owns dependency assembly: it opens the database, constructs every
// repository, service and handler, and exposes the Echo instance. cmd/server
// stays a thin launcher; everything that knows how the objects fit together
// lives here.
package app

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"relationship/internal/ai"
	"relationship/internal/backup"
	"relationship/internal/config"
	"relationship/internal/db"
	"relationship/internal/handler"
	"relationship/internal/repository"
	"relationship/internal/service"
)

// App is the assembled application. Handlers are exported so the router can
// wire them to routes; everything else is internal wiring detail.
type App struct {
	Config   *config.Config
	CfgPath  string
	DB       *sql.DB
	VecReady bool
	ingest   *service.IngestService

	personHandler   *handler.PersonHandler
	eventHandler    *handler.EventHandler
	traitHandler    *handler.TraitHandler
	aiHandler       *handler.AIHandler
	configHandler   *handler.ConfigHandler
	orgHandler      *handler.OrganizationHandler
	followUpHandler *handler.FollowUpHandler
	adviceHandler   *handler.AdviceHandler
	reportHandler   *handler.ReportHandler
	backupHandler   *handler.BackupHandler
	auditHandler    *handler.AuditHandler
	backupSvc       *backup.Service
}

// New opens the database, runs migrations and wires every layer. It returns an
// error instead of exiting so the caller decides how to report a failed start.
func New(cfg *config.Config, cfgPath string) (*App, error) {
	// A staged restore swaps the database file before the first connection
	// exists — the only moment the swap is atomic from the app's perspective.
	if applied, err := backup.ApplyPendingRestore(cfg.Database.Path); err != nil {
		log.Printf("WARNING: staged restore was not applied: %v", err)
	} else if applied {
		log.Printf("staged restore applied to %s", cfg.Database.Path)
	}

	database, err := db.OpenDB(cfg.Database.Path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Migrate(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	a := &App{Config: cfg, CfgPath: cfgPath, DB: database}
	if err := a.wire(database); err != nil {
		database.Close()
		return nil, err
	}
	return a, nil
}

// wire constructs repositories, services and handlers in dependency order.
func (a *App) wire(database *sql.DB) error {
	cfg := a.Config

	personRepo := repository.NewPersonRepo(database)
	eventRepo := repository.NewEventRepo(database)
	traitRepo := repository.NewTraitRepo(database)
	vecRepo := repository.NewVecRepo(database)
	orgRepo := repository.NewOrganizationRepo(database)
	followUpRepo := repository.NewFollowUpRepo(database)

	if err := vecRepo.Init(cfg.LLM.EmbedDim); err != nil {
		// Semantic retrieval is optional; recency and profile context still work.
		log.Printf("WARNING: vector index unavailable: %v", err)
	} else {
		a.VecReady = true
	}

	aiClient := ai.NewClient(&cfg.LLM)

	personService := service.NewPersonService(personRepo, vecRepo, orgRepo)
	// Seed the reserved “我” person so records can name the user as their
	// subject or a participant from the first launch. A failed seed must not
	// block startup; the next launch retries.
	if err := personService.EnsureSelf(); err != nil {
		log.Printf("WARNING: seed self person failed: %v", err)
	}
	orgService := service.NewOrganizationService(orgRepo)
	eventService := service.NewEventService(eventRepo, vecRepo, personService, traitRepo)
	aiService := service.NewAIService(&cfg.LLM, vecRepo, traitRepo, eventRepo, personRepo, aiClient)
	ingestService := service.NewIngestService(aiService, &cfg.LLM)
	a.ingest = ingestService
	followUpService := service.NewFollowUpService(followUpRepo, personService, eventRepo)
	adviceService := service.NewAdviceService(repository.NewAdviceRepo(database), eventRepo, followUpRepo, personService, aiService)
	reportService := service.NewReportService(
		repository.NewReportRepo(database), eventRepo, followUpRepo,
		personService, aiService,
	)

	a.personHandler = handler.NewPersonHandler(personService)
	a.eventHandler = handler.NewEventHandler(eventService, aiService, ingestService)
	a.traitHandler = handler.NewTraitHandler(traitRepo)
	a.aiHandler = handler.NewAIHandler(aiService)
	a.configHandler = handler.NewConfigHandler(cfg, a.CfgPath)
	a.orgHandler = handler.NewOrganizationHandler(orgService)
	a.followUpHandler = handler.NewFollowUpHandler(followUpService)
	a.adviceHandler = handler.NewAdviceHandler(adviceService)
	a.reportHandler = handler.NewReportHandler(reportService)

	// Data lifecycle: scheduled snapshots with sane fallbacks when the config
	// section is absent or nonsensical.
	bcfg := cfg.Backup
	if bcfg.Dir == "" {
		bcfg.Dir = "backups"
	}
	if bcfg.IntervalHours <= 0 {
		bcfg.IntervalHours = 24
	}
	if bcfg.Keep <= 0 {
		bcfg.Keep = 14
	}
	a.backupSvc = backup.NewService(database, cfg.Database.Path, bcfg.Dir,
		time.Duration(bcfg.IntervalHours)*time.Hour, bcfg.Keep, bcfg.Passphrase)
	if bcfg.Enabled {
		a.backupSvc.Start()
	}
	a.backupHandler = handler.NewBackupHandler(a.backupSvc)
	a.auditHandler = handler.NewAuditHandler(database)

	// Data hygiene: a startup pass removes vectors orphaned by crashes and
	// trims the audit log when a retention window is configured.
	maintenanceSvc := service.NewMaintenanceService(database, vecRepo, cfg.Maintenance.AuditRetentionDays)
	if report, err := maintenanceSvc.Cleanup(); err != nil {
		log.Printf("WARNING: startup maintenance pass failed: %v", err)
	} else if report.OrphanVectors > 0 || report.AuditRowsRemoved > 0 {
		log.Printf("startup maintenance: removed %d orphan vector(s), %d expired audit row(s)",
			report.OrphanVectors, report.AuditRowsRemoved)
	}

	// Startup recovery: records stored but never extracted (crash, shutdown
	// with a full queue) go back into the queue. Pending rows are the source
	// of truth; the in-memory queue only holds ordering.
	if n, err := ingestService.RecoverPending(); err != nil {
		log.Printf("WARNING: pending record recovery failed: %v", err)
	} else if n > 0 {
		log.Printf("re-enqueued %d pending record(s) for extraction", n)
	}
	return nil
}

// Close releases the database connection, the extraction queue and the
// backup schedule.
func (a *App) Close() error {
	if a.ingest != nil {
		a.ingest.Stop()
	}
	if a.backupSvc != nil {
		a.backupSvc.Stop()
	}
	return a.DB.Close()
}

// WarnIfExposed prints a startup warning when the configured host makes the
// service reachable beyond this machine. Relationship notes are sensitive; a
// LAN-visible instance should always be a deliberate choice, not an accident.
func (a *App) WarnIfExposed() {
	host := a.Config.Server.Host
	switch host {
	case "localhost", "127.0.0.1", "::1", "":
		return
	}
	if a.Config.Server.AuthToken != "" {
		log.Printf("WARNING: server is bound to %q, which is reachable from other machines. "+
			"Access control is enabled, but the data still leaves the machine on every request — "+
			"keep host: localhost unless the LAN exposure is deliberate.", host)
		return
	}
	log.Printf("WARNING: server is bound to %q, which is reachable from other machines. "+
		"Access control is OFF: anyone on this network can read and edit your relationship data. "+
		"Set server.auth_token in config.yaml, or keep host: localhost.", host)
}
