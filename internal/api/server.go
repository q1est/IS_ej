package api

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"school-journal/internal/admin"
	"school-journal/internal/auth"
	"school-journal/internal/behavior"
	"school-journal/internal/config"
	"school-journal/internal/grades"
	"school-journal/internal/platform/httpx"
	"school-journal/internal/platform/ratelimit"
	studentsmodule "school-journal/internal/students"
)

func New(cfg config.Config, db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	authHandler := auth.New(db, cfg)
	protect := authHandler.Protect
	limiter := ratelimit.New(10, time.Minute)
	authHandler.Register(mux, limiter)
	studentsmodule.New(db).Register(mux, protect)
	grades.New(db).Register(mux, protect)
	behavior.New(db).Register(mux, protect)
	admin.New(db).Register(mux, protect)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			httpx.Error(w, http.StatusServiceUnavailable, "db_unavailable", "Database unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowed[strings.TrimSpace(origin)] = struct{}{}
	}
	return securityMiddleware(allowed, mux)
}

func securityMiddleware(allowed map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; !ok {
				httpx.Error(w, http.StatusForbidden, "origin_denied", "Origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
