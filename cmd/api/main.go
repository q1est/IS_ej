package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"school-journal/internal/api"
	"school-journal/internal/config"
	"school-journal/internal/platform/postgres"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	startup, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := postgres.Open(startup, cfg.DatabaseURL, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	cancelStartup()
	if err != nil {
		log.Fatalf("database startup failed: %v", err)
	}
	defer db.Close()
	server := &http.Server{
		Addr: ":" + cfg.Port, Handler: api.New(cfg, db),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()
	log.Printf("listening on :%s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http server failed: %v", err)
	}
}
