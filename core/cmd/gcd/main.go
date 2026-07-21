// Package main is the entry point of the Lanos Go Core Daemon (gcd).
// gcd runs as an independent background process on desktop platforms,
// serving a local REST API consumed by the Flutter UI.
//
// Lifecycle (see PRD §5.1.2 - §5.1.5):
//  1. Acquire single-instance lock; if held, exit with `{"already_running":true,...}`.
//  2. Generate 32-byte random API_TOKEN (in-memory only, never persisted).
//  3. Bind a random port in range 52100-52999 (or persisted from config).
//  4. Emit one JSON line to stdout: {"port":52103,"api_token":"<base64>","version":"0.1.0"}.
//  5. Start mDNS broadcast (discovery package).
//  6. Serve REST API on 127.0.0.1 only.
//  7. Handle SIGINT/SIGTERM (Linux/macOS) / Ctrl-Break (Windows) for graceful shutdown.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lanos/lanos/core/api"
	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/discovery"
	"github.com/lanos/lanos/core/identity"
	"github.com/lanos/lanos/core/instance"
	"github.com/lanos/lanos/core/lifecycle"
	"github.com/lanos/lanos/core/store"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gcd: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 1. Single-instance lock.
	lock, err := instance.Acquire()
	if err != nil {
		return fmt.Errorf("acquire instance lock: %w", err)
	}
	defer lock.Release()

	// 2. Load config + identity.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ident, err := identity.LoadOrCreate()
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	// 3. Open transfer log DB.
	dataDir, err := cfg.DataDir()
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	db, err := store.Open(dataDir)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	// 4. Generate Bearer token (in-memory only, never persisted).
	token, err := lifecycle.NewToken()
	if err != nil {
		return fmt.Errorf("generate token: %w", err)
	}

	// 5. Bind to 127.0.0.1:<port>. If cfg.Port==0 a random port in
	// 52100-52999 is chosen; the chosen port is persisted to config.yaml
	// (PRD §5.1.2: "端口随机持久化") so subsequent launches reuse it.
	port, listener, err := lifecycle.BindLocal(cfg.Port)
	if err != nil {
		return fmt.Errorf("bind local: %w", err)
	}
	defer listener.Close()
	if cfg.Port != port {
		cfg.Port = port
		if err := cfg.Save(); err != nil {
			// Non-fatal: we still serve on the bound port; next launch
			// will just pick a new random port.
			fmt.Fprintf(os.Stderr, "gcd: warn: persist port: %v\n", err)
		}
	}

	// 6. Start mDNS discovery. Must happen AFTER the port is known so the
	// TXT `port=` field advertises the real listening port.
	disc, err := discovery.New(cfg, ident)
	if err != nil {
		return fmt.Errorf("init discovery: %w", err)
	}
	if err := disc.Start(); err != nil {
		return fmt.Errorf("start discovery: %w", err)
	}
	defer disc.Stop()

	// 7. Build API server with discovery wired in (for /api/v1/devices).
	srv := api.NewServer(api.Config{
		Version:     version,
		Token:       token,
		Config:      cfg,
		DB:          db,
		Discovery:   disc,
		EventSource: disc,
	})

	// 8. Handshake: emit JSON to stdout for Flutter to read.
	if err := lifecycle.EmitHandshake(port, token, version); err != nil {
		return fmt.Errorf("emit handshake: %w", err)
	}

	// 9. Serve until shutdown.
	return srv.Serve(ctx, listener)
}
