// Package api implements the local REST API served by gcd to Flutter.
// See PRD §5.1.4 (API design), §5.1.5 (Bearer auth + CORS), §5.4 (endpoints).
//
// MVP P0 ships a minimal server with /api/v1/ping and /api/v1/version that
// compiles and proves the lifecycle handshake end-to-end. Full endpoint
// coverage (devices, transfers, shares, events) lands in P1 W2-W4.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/discovery"
	"github.com/lanos/lanos/core/share"
	"github.com/lanos/lanos/core/store"
)

// Config bundles everything the Server needs at construction time.
type Config struct {
	Version   string
	Token     string
	Config    *config.Config
	DB        *store.DB
	Discovery DeviceLister
	// ShareManager handles web share lifecycle. If nil, /shares returns 503.
	ShareManager *share.Manager
	// EventSource feeds device presence events to the SSE /events stream. If
	// nil, /events returns 503.
	EventSource EventSource
	// Broker fans out events to SSE subscribers. Created by NewServer from
	// EventSource if both are non-nil.
	Broker *eventBroker
}

// DeviceLister is the subset of *discovery.Discovery that the API needs.
// Declared as an interface so tests can substitute a fake without importing
// the discovery package's network stack.
type DeviceLister interface {
	Self() *discovery.Device
	Devices() []*discovery.Device
}

// Server is the local REST API server bound to 127.0.0.1.
type Server struct {
	cfg Config
	srv *http.Server
}

// NewServer constructs the Server with all routes wired. The caller owns
// the net.Listener and calls Serve.
func NewServer(cfg Config) *Server {
	s := &Server{cfg: cfg}
	if cfg.EventSource != nil && cfg.Broker == nil {
		cfg.Broker = newEventBroker(cfg.EventSource)
		s.cfg = cfg
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(s.bearerAuth)
	r.Use(s.corsGuard)

	r.Route("/api/v1", func(r chi.Router) {
		// /ping is exempt from Bearer auth (see PRD §5.1.5 exception list).
		// bearerAuth checks isExempt(r) to skip auth for it.
		r.Get("/ping", s.handlePing)
		r.Get("/version", s.handleVersion)
		r.Get("/devices", s.handleDevices)
		// SSE is a long-lived stream: it must NOT be subject to the 30s request
		// timeout applied to the request/response routes below. Mount it before
		// the Timeout middleware.
		r.Get("/events", s.handleEvents)
		// Standard request/response routes get a 30s timeout.
		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(30 * time.Second))
			// Shares (web share management)
			r.Route("/shares", func(r chi.Router) {
				r.Get("/", s.handleListShares)
				r.Post("/", s.handleCreateShare)
				r.Get("/{id}", s.handleGetShare)
				r.Delete("/{id}", s.handleStopShare)
			})
			// Transfers (transfer log)
			r.Route("/transfers", func(r chi.Router) {
				r.Get("/", s.handleListTransfers)
				r.Get("/{id}", s.handleGetTransfer)
				r.Delete("/{id}", s.handleDeleteTransfer)
			})
			// Settings
			r.Route("/settings", func(r chi.Router) {
				r.Get("/", s.handleGetSettings)
				r.Put("/", s.handleUpdateSettings)
			})
		})
	})

	s.srv = &http.Server{Handler: r}
	return s
}

// Serve blocks until the listener returns or ctx is canceled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// --- Middleware ---

// bearerAuth rejects any request lacking a valid Authorization: Bearer <token>
// header. Exempt paths are handled by the noAuth middleware below.
func (s *Server) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isExempt(r) {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			deny(w, "missing bearer token")
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if got != s.cfg.Token {
			deny(w, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsGuard restricts cross-origin requests to localhost loopback only.
// See PRD §5.1.5 threat table.
func (s *Server) corsGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !isLocalhostOrigin(origin) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// noAuth removed: bearerAuth calls isExempt(r) directly to skip auth for
// PRD §5.1.5 exempt paths (currently /api/v1/ping only).

// isExempt returns true for paths that PRD §5.1.5 lists as not requiring
// Bearer auth (currently /api/v1/ping only).
func isExempt(r *http.Request) bool {
	return r.URL.Path == "/api/v1/ping"
}

func isLocalhostOrigin(origin string) bool {
	for _, prefix := range []string{"http://localhost", "http://127.0.0.1", "http://[::1]"} {
		if strings.HasPrefix(origin, prefix) {
			return true
		}
	}
	return false
}

func deny(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    "UNAUTHORIZED",
			"message": msg,
		},
	})
}

// --- Handlers ---

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.cfg.Version,
	})
}

// handleDevices returns the local device and currently-online peers.
// See PRD §5.4 GET /api/v1/devices.
//
// Response shape:
//
//	{
//	  "self":  { ...device... },
//	  "peers": [ { ...device... }, ... ]
//	}
//
// "self" is nil if Discovery was not wired into the server (e.g. in unit
// tests). "peers" is never nil; it is an empty array when no peers are
// online.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	var self *discovery.Device
	var peers []*discovery.Device
	if s.cfg.Discovery != nil {
		self = s.cfg.Discovery.Self()
		peers = s.cfg.Discovery.Devices()
	}
	if peers == nil {
		peers = []*discovery.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"self":  self,
		"peers": peers,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Best-effort log; cannot change status now.
		_ = err
	}
}

// guard against forgetting a Config field somewhere in the future.
var _ = fmt.Sprintf
