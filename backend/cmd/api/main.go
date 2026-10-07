package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"time"

	"blog-system/backend/internal/dbschema"
	"blog-system/backend/internal/httpapi"
	"blog-system/backend/internal/mysqlrepo"
	"blog-system/backend/internal/seed"
	_ "github.com/go-sql-driver/mysql"
)

func getenv(k, v string) string {
	if x := os.Getenv(k); x != "" {
		return x
	}
	return v
}
func main() {
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	dsn := getenv("DB_DSN", "blog:blog@tcp(127.0.0.1:3306)/blog?parseTime=true&loc=UTC")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Error("database open", "error", err)
		os.Exit(1)
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
			slog.Error("database unavailable", "error", err)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	if mode == "migrate" {
		if err = dbschema.Migrate(db); err != nil {
			slog.Error("migration failed", "error", err)
			os.Exit(1)
		}
		if err = seed.Demo(ctx, db, getenv("UPLOAD_DIR", "./uploads"), getenv("DEMO_PASSWORD", "Demo12345!")); err != nil {
			slog.Error("seed failed", "error", err)
			os.Exit(1)
		}
		slog.Info("migration and seed complete")
		return
	}
	if mode != "serve" {
		slog.Error("unknown mode", "mode", mode)
		os.Exit(1)
	}
	store := mysqlrepo.Store{DB: db}
	uploadDir := getenv("UPLOAD_DIR", "./uploads")
	go func() {
		for {
			maintenanceContext, done := context.WithTimeout(context.Background(), 30*time.Second)
			if err := store.Cleanup(maintenanceContext, uploadDir); err != nil {
				slog.Error("maintenance failed", "error", err)
			}
			done()
			time.Sleep(24 * time.Hour)
		}
	}()
	handler := httpapi.New(store, uploadDir, getenv("PUBLIC_ORIGIN", "http://localhost:8080"), os.Getenv("ADDITIONAL_ORIGINS"))
	server := &http.Server{Addr: getenv("LISTEN_ADDR", ":8081"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("api listening", "address", server.Addr)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
