// Command server runs the Signet platform API.
package main

import (
	"context"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/oleary-labs/signet-platform/backend/internal/api"
	"github.com/oleary-labs/signet-platform/backend/internal/auth"
	"github.com/oleary-labs/signet-platform/backend/internal/chain"
	"github.com/oleary-labs/signet-platform/backend/internal/config"
	"github.com/oleary-labs/signet-platform/backend/internal/db"
	"github.com/oleary-labs/signet-platform/backend/internal/storage"
)

// initLogging points slog at stderr and, when a logs directory can be created,
// also at an appending file. On an ephemeral filesystem the file resets on
// redeploy, but stderr is still captured by the platform's log viewer.
func initLogging() {
	var out io.Writer = os.Stderr
	if err := os.MkdirAll("logs", 0o755); err == nil {
		if f, ferr := os.OpenFile(filepath.Join("logs", "platform.log"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			out = io.MultiWriter(os.Stderr, f)
		}
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})))
}

func main() {
	initLogging()
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// In production assets live in a private S3 bucket; in development they go
	// to the local filesystem, so a developer needs no S3 to run the stack.
	var assets *storage.Store
	if cfg.InProduction {
		assets, err = storage.New(ctx, cfg)
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
	} else if err := os.MkdirAll(cfg.LocalUploadDir, 0o755); err != nil {
		log.Fatalf("local upload dir: %v", err)
	}

	chainClient := chain.New(cfg.RPCURL, cfg.FactoryAddress, cfg.ChainID)
	if !chainClient.Enabled() {
		slog.Warn("chain reads are not configured — group state and the node registry will not sync",
			"rpc_url_set", cfg.RPCURL != "", "factory_set", cfg.FactoryAddress != "")
	}

	// Certificates are how a developer who signed in with a wallet still ends
	// up holding a Signet key. Without the platform auth key the console can
	// still run; it just cannot provision keys.
	certs, err := auth.NewCertificateSigner(cfg.PlatformAuthKey, cfg.BootstrapGroup, time.Hour)
	if err != nil {
		log.Fatalf("certificate signer: %v", err)
	}
	if certs == nil {
		slog.Warn("no platform auth key configured — developers cannot be issued Signet keys")
	} else {
		slog.Info("certificate signer ready", "auth_key_pub", certs.PublicKey(), "group", certs.GroupID())
	}

	srv := api.New(cfg, pool, assets, chainClient, certs)

	// Reconcile staff with the environment. Sign-in grants on its own, so this
	// exists mainly for the other direction: a subject removed from the list
	// loses the flag here rather than keeping it until someone notices.
	if granted, revoked, err := srv.Store().SyncStaffBySubject(ctx, cfg.StaffSubjects); err != nil {
		slog.Error("staff reconciliation failed", "error", err)
	} else if granted > 0 || revoked > 0 {
		slog.Info("reconciled platform staff", "granted", granted, "revoked", revoked)
	}

	startBackgroundJobs(ctx, cfg, srv)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", ":"+cfg.Port, "env", cfg.Env, "chain_id", cfg.ChainID)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	slog.Info("shutdown complete")
}

// startBackgroundJobs launches the periodic work that keeps the platform's
// caches current: node liveness probes, the on-chain registry and group sync,
// and retention pruning.
//
// Each job runs once at boot so a freshly started server is useful
// immediately, rather than showing an empty marketplace until the first tick.
func startBackgroundJobs(ctx context.Context, cfg *config.Config, srv *api.Server) {
	if cfg.HealthProbeSecs > 0 {
		go loop(ctx, "node health probe", time.Duration(cfg.HealthProbeSecs)*time.Second, func(ctx context.Context) error {
			return srv.ProbeNodes(ctx)
		})
	}

	if cfg.ChainSyncSecs > 0 {
		go loop(ctx, "node registry sync", time.Duration(cfg.ChainSyncSecs)*time.Second, func(ctx context.Context) error {
			return srv.SyncNodeRegistry(ctx)
		})
		go loop(ctx, "group state sync", time.Duration(cfg.ChainSyncSecs)*time.Second, func(ctx context.Context) error {
			return srv.SyncAllGroups(ctx)
		})
	}

	// Retention. Hourly is often enough for rows that age out over days, and
	// cheap enough that it never contends with request traffic.
	go loop(ctx, "retention prune", time.Hour, func(ctx context.Context) error {
		db := srv.Store()
		if _, err := db.PurgeExpiredChallenges(ctx); err != nil {
			return err
		}
		if _, err := db.PruneHealthSamples(ctx); err != nil {
			return err
		}
		_, err := db.PruneDeliveries(ctx)
		return err
	})
}

// loop runs fn immediately and then on an interval, logging failures without
// stopping — a transient RPC outage must not permanently kill a sync job.
func loop(ctx context.Context, name string, every time.Duration, fn func(context.Context) error) {
	run := func() {
		runCtx, cancel := context.WithTimeout(ctx, every+30*time.Second)
		defer cancel()
		start := time.Now()
		if err := fn(runCtx); err != nil {
			slog.Error("background job failed", "job", name, "error", err, "elapsed_ms", time.Since(start).Milliseconds())
			return
		}
		slog.Debug("background job completed", "job", name, "elapsed_ms", time.Since(start).Milliseconds())
	}

	run()
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
