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
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/lanos/lanos/core/api"
	"github.com/lanos/lanos/core/config"
	"github.com/lanos/lanos/core/discovery"
	"github.com/lanos/lanos/core/identity"
	"github.com/lanos/lanos/core/instance"
	"github.com/lanos/lanos/core/lifecycle"
	corenet "github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/receive"
	"github.com/lanos/lanos/core/share"
	"github.com/lanos/lanos/core/store"
	"github.com/lanos/lanos/core/transfer"
	"github.com/lanos/lanos/core/transport"
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
		if errors.Is(err, instance.ErrAlreadyRunning) {
			lifecycle.EmitHandshakeAlreadyRunning(version)
		}
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

	// 6b. Linux: warn if Avahi daemon is missing (mDNS will silently fail).
	if runtime.GOOS == "linux" {
		if err := corenet.CheckAvahi(); err != nil {
			fmt.Fprintf(os.Stderr, "gcd: warn: %v\n", err)
		}
		if hint := corenet.CheckFirewall(); hint != "" {
			fmt.Fprintf(os.Stderr, "gcd: warn: %s\n", hint)
		}
	}

	// 6c. Web share server: binds a second listener (dual-stack) so LAN
	// peers can reach it. Uses the same port as the API when possible —
	// the API binds 127.0.0.1 only, so the share listener binds 0.0.0.0
	// on a separate port from the 52100-52999 range.
	shareMgr := share.NewManager(share.MaxShares)
	shareLn, err := net.Listen("tcp", fmt.Sprintf(":%d", port+1))
	if err != nil {
		// Port conflict: fall back to a random port.
		shareLn, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("bind share listener: %w", err)
		}
	}
	shareSrv := share.NewServer(shareMgr, shareLn)
	go func() {
		if err := shareSrv.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gcd: share server: %v\n", err)
		}
	}()
	defer shareSrv.Close() // StopAll: all share links die with the process.
	defer shareMgr.Stop()  // terminate cleanupLoop goroutine

	// 6d. Derive X25519 static keys from ed25519 identity for Noise XX.
	staticKeys, err := transport.DeriveStaticKeys(ident.PrivED)
	if err != nil {
		return fmt.Errorf("derive static keys: %w", err)
	}

	// 6e. Transfer and receive managers for live transfer state.
	transferMgr := transfer.NewManager(transfer.DefaultMaxConcurrent)
	receiveMgr := receive.NewManager(receive.DefaultMaxConcurrent)
	receiveMgr.StartExpiryLoop(ctx.Done(), 0)

	// 6f. Inbound P2P transfer listener (P1-8).
	inboundSrv, err := receive.NewInboundServer(fmt.Sprintf(":%d", port+2), receiveMgr, staticKeys)
	if err != nil {
		// Port conflict: fall back to a random port, same as shareLn above.
		slog.Warn("inbound: preferred port busy, falling back to random", "preferred", port+2)
		inboundSrv, err = receive.NewInboundServer(":0", receiveMgr, staticKeys)
		if err != nil {
			return fmt.Errorf("inbound p2p: %w", err)
		}
	}
	inboundSrv.Start()
	defer inboundSrv.Close()
	slog.Info("inbound p2p listener", "addr", inboundSrv.Addr())

	// 7. Build API server with discovery wired in (for /api/v1/devices).
	srv := api.NewServer(api.Config{
		Version:      version,
		Token:        token,
		Config:       cfg,
		DB:           db,
		Discovery:    disc,
		ShareManager: shareMgr,
		SharePort:    shareSrv.Port(),
		TransferMgr:  transferMgr,
		ReceiveMgr:   receiveMgr,
		StaticKeys:   staticKeys,
		EventSource:  disc,
	})

	// 8. Handshake: emit JSON to stdout for the launcher to read.
	if err := lifecycle.EmitHandshake(port, token, version); err != nil {
		return fmt.Errorf("emit handshake: %w", err)
	}

	// 9. Open Web UI in default browser.
	uiURL := fmt.Sprintf("http://127.0.0.1:%d/ui?token=%s", port, token)
	go openBrowser(uiURL)

	// 10. Serve until shutdown.
	return srv.Serve(ctx, listener)
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		fmt.Fprintf(os.Stderr, "gcd: warn: open browser: %v\n", err)
	}
}
