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

	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/discovery"
	"github.com/lanos/lanos/core/receive"
	"github.com/lanos/lanos/core/share"
	"github.com/lanos/lanos/core/store"
	"github.com/lanos/lanos/core/transfer"
	"github.com/lanos/lanos/core/transport"
	"github.com/lanos/lanos/core/web"
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
	// SharePort is the port the share HTTP server listens on, used to build
	// download URLs returned by POST /shares.
	SharePort int
	// TransferMgr manages outgoing file transfers. If nil, transfer-related
	// live endpoints return 503.
	TransferMgr *transfer.Manager
	// ReceiveMgr manages incoming file transfer prompts. If nil, incoming
	// endpoints return 503.
	ReceiveMgr *receive.Manager
	// StaticKeys is the local device's X25519 keypair derived from the
	// ed25519 identity. Required for POST /transfers to initiate the Noise
	// XX handshake when dialing a peer.
	StaticKeys transport.StaticKeys
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
	cfg    Config
	srv    *http.Server
	appCtx context.Context
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

	// Web UI: serve embedded SPA at /ui/. The page receives the API token
	// via query param (?token=...) so it can call authenticated endpoints.
	r.Get("/ui", s.handleWebUI)
	r.Get("/ui/", s.handleWebUI)

	r.Use(s.bearerAuth)
	r.Use(s.corsGuard)

	r.Route("/api/v1", func(r chi.Router) {
		// /ping is exempt from Bearer auth (see PRD §5.1.5 exception list).
		// bearerAuth checks isExempt(r) to skip auth for it.
		r.Get("/ping", s.handlePing)
		r.Get("/version", s.handleVersion)
		r.Get("/devices", s.handleDevices)
		r.Get("/diagnostics", s.handleDiagnostics)
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
				r.Get("/history", s.handleListShareHistory)
				r.Get("/export", s.handleExportShares)
				r.Get("/{id}", s.handleGetShare)
				r.Delete("/{id}", s.handleStopShare)
			})
			// Transfers (transfer log + initiate send)
			r.Route("/transfers", func(r chi.Router) {
				r.Get("/", s.handleListTransfers)
				r.Post("/", s.handleCreateTransfer)
				r.Get("/export", s.handleExportTransfers)
				r.Get("/{id}", s.handleGetTransfer)
				r.Post("/{id}/cancel", s.handleCancelTransfer)
				r.Delete("/{id}", s.handleDeleteTransfer)
			})
			// Incoming transfer prompts
			r.Route("/incoming", func(r chi.Router) {
				r.Get("/", s.handleListIncoming)
				r.Post("/{id}/accept", s.handleAcceptIncoming)
				r.Post("/{id}/reject", s.handleRejectIncoming)
				r.Post("/{id}/cancel", s.handleCancelIncoming)
			})
			// Settings
			r.Route("/settings", func(r chi.Router) {
				r.Get("/", s.handleGetSettings)
				r.Put("/", s.handleUpdateSettings)
				r.Post("/", s.handleUpdateSettings)
			})
		})
	})

	s.srv = &http.Server{Handler: r}
	return s
}

func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	data, err := web.Assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, "UI not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

// Serve blocks until the listener returns or ctx is canceled.
// ctx is stored on s as the application-scoped context and used by background
// goroutines spawned by HTTP handlers (e.g. async file transfers). These must
// NOT use r.Context(), which is canceled the moment the handler returns.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.appCtx = ctx
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
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
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

// handleDiagnostics returns a snapshot of the local network stack for
// debugging dual-stack / IPv6 reachability issues. The response includes
// the detected IP version capability and per-interface addresses.
//
// Response shape:
//
//	{
//	  "ip_version": "46" | "4" | "6",
//	  "interfaces": [ InterfaceInfo, ... ],
//	  "source_ips": [ "192.168.1.5", "fd00::5", ... ]
//	}
func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ip_version": discovery.LocalIPVersion(),
		"interfaces": discovery.Interfaces(),
		"source_ips": discovery.LocalSourceIPs(),
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
