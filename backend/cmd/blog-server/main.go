package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"blog-system/backend/internal/bloghttp"
	"blog-system/backend/internal/dbschema"
	"blog-system/backend/internal/mysqlrepo"
	"blog-system/backend/internal/observ"
	"blog-system/backend/internal/seed"
	_ "github.com/go-sql-driver/mysql"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func getenv(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func main() {
	slog.SetDefault(slog.New(observ.NewLogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}), prometheus.DefaultRegisterer)))
	if err := run(); err != nil {
		slog.Error("blog server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	dsn := getenv("DB_DSN", "blog:blog@tcp(127.0.0.1:3306)/blog?parseTime=true&loc=UTC")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return fmt.Errorf("database unavailable: %w", err)
		}
		time.Sleep(time.Second)
	}
	if mode == "migrate" {
		if err = dbschema.Migrate(db); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
		if err = seed.Demo(ctx, db, getenv("UPLOAD_DIR", "./uploads"), getenv("DEMO_PASSWORD", "Demo12345!")); err != nil {
			return fmt.Errorf("seed failed: %w", err)
		}
		slog.Info("migration and seed complete")
		return nil
	}
	if mode == "demo-data" {
		if err := seed.Extra(ctx, db, getenv("DEMO_PASSWORD", "Demo12345!")); err != nil {
			return fmt.Errorf("extra demo data failed: %w", err)
		}
		slog.Info("extra demo data complete")
		return nil
	}
	if mode != "serve" {
		return fmt.Errorf("unknown mode %q", mode)
	}
	for _, metric := range []struct {
		name  string
		help  string
		value func(sql.DBStats) float64
	}{
		{"blog_db_connections_open", "Open connections in the blog server database pool.", func(s sql.DBStats) float64 { return float64(s.OpenConnections) }},
		{"blog_db_connections_in_use", "Connections currently in use in the blog server database pool.", func(s sql.DBStats) float64 { return float64(s.InUse) }},
		{"blog_db_connections_idle", "Idle connections in the blog server database pool.", func(s sql.DBStats) float64 { return float64(s.Idle) }},
		{"blog_db_connections_max", "Maximum open connections allowed in the blog server database pool.", func(s sql.DBStats) float64 { return float64(s.MaxOpenConnections) }},
	} {
		metric := metric
		prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: metric.name, Help: metric.help}, func() float64 {
			return metric.value(db.Stats())
		}))
	}
	prometheus.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "blog_db_connection_waits_total", Help: "Total waits for an blog server database connection.",
	}, func() float64 { return float64(db.Stats().WaitCount) }))
	sampleRatio, err := strconv.ParseFloat(getenv("TRACE_SAMPLE_RATIO", "1"), 64)
	if err != nil {
		return fmt.Errorf("invalid TRACE_SAMPLE_RATIO: %w", err)
	}
	shutdownTracer, err := observ.InitTracer(context.Background(), os.Getenv("OTLP_ENDPOINT"), sampleRatio)
	if err != nil {
		return fmt.Errorf("tracer init failed: %w", err)
	}
	defer func() {
		shutdownContext, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := shutdownTracer(shutdownContext); err != nil {
			observ.LogFailure(shutdownContext, "tracer shutdown failed", err, "event", "tracer_shutdown")
		}
	}()
	debugRaw := strings.EqualFold(os.Getenv("OBS_DEBUG_RAW"), "true")
	store := mysqlrepo.NewStore(db, debugRaw)
	uploadDir := getenv("UPLOAD_DIR", "./uploads")
	go func() {
		for {
			maintenanceContext, done := context.WithTimeout(context.Background(), 30*time.Second)
			if err := store.Cleanup(maintenanceContext, uploadDir); err != nil {
				observ.LogFailure(maintenanceContext, "maintenance failed", err, "event", "maintenance")
			}
			done()
			time.Sleep(24 * time.Hour)
		}
	}()
	handler := bloghttp.New(store, uploadDir, getenv("PUBLIC_ORIGIN", "http://localhost:8080"), os.Getenv("ADDITIONAL_ORIGINS"), debugRaw)
	server := &http.Server{Addr: getenv("LISTEN_ADDR", ":8081"), Handler: otelhttp.NewHandler(handler, "http.server"), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	metricsServer := &http.Server{Addr: getenv("METRICS_ADDR", "127.0.0.1:9090"), Handler: metricsMux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	serveErrors := make(chan error, 2)
	go func() { serveErrors <- server.ListenAndServe() }()
	go func() { serveErrors <- metricsServer.ListenAndServe() }()
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("blog server listening", "address", server.Addr)
	slog.Info("metrics listening", "address", metricsServer.Addr)
	select {
	case <-signalContext.Done():
	case err = <-serveErrors:
	}
	shutdownContext, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if shutdownErr := server.Shutdown(shutdownContext); shutdownErr != nil {
		return fmt.Errorf("blog server shutdown: %w", shutdownErr)
	}
	if shutdownErr := metricsServer.Shutdown(shutdownContext); shutdownErr != nil {
		return fmt.Errorf("metrics shutdown: %w", shutdownErr)
	}
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}
