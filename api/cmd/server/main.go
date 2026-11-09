// Command server runs the Tenggat API. "server healthcheck" asks a running
// server for /health and exits 0 or 1; the Docker HEALTHCHECK uses it because
// the runtime image has no shell or curl.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mrramdn/tenggat/api/internal/cache"
	"github.com/mrramdn/tenggat/api/internal/httpapi"
	"github.com/mrramdn/tenggat/api/internal/store"
	"github.com/mrramdn/tenggat/api/internal/tlscheck"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("api berhenti", "galat", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := getenv("PORT", "8080")
	interval, err := time.ParseDuration(getenv("CEK_INTERVAL", "6h"))
	if err != nil || interval < time.Minute {
		return errors.New("CEK_INTERVAL harus durasi seperti 6h dan minimal 1m")
	}

	db, err := connect(ctx, log)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	api := &httpapi.Server{Store: db, Checker: tlscheck.New(), Now: time.Now, Log: log}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		rc := cache.New(addr)
		defer rc.Close()
		api.Cache = rc
		log.Info("cache redis aktif", "alamat", addr)
	}
	if v := os.Getenv("BATAS_PER_MENIT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return errors.New("BATAS_PER_MENIT harus bilangan bulat positif")
		}
		api.LimitPerMinute = n
	}
	go api.RunRechecks(ctx, interval)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Info("api siap", "port", port, "cek_interval", interval.String())
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// connect opens the pool and waits up to 30 seconds for PostgreSQL to answer.
func connect(ctx context.Context, log *slog.Logger) (*store.Store, error) {
	db, err := store.Open(ctx)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = db.Ping(pingCtx)
		cancel()
		if err == nil {
			return db, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			db.Close()
			return nil, err
		}
		log.Info("menunggu database", "galat", err)
		time.Sleep(2 * time.Second)
	}
}

func healthcheck() int {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + getenv("PORT", "8080") + "/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
