package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lanos/lanos/core/discovery"
	lanosnet "github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/share"
	"github.com/lanos/lanos/core/store"
	"github.com/lanos/lanos/core/usecase"
)

// ErrIncompatibleIPVersion is returned when a peer advertises only addresses
// of an IP family the local host cannot reach (e.g. v6-only peer, v4-only
// local). Surface as HTTP 503 with code INCOMPATIBLE_IP_VERSION.
var ErrIncompatibleIPVersion = errors.New("INCOMPATIBLE_IP_VERSION")

// ErrNoPeerAddress is returned when a peer advertises no address at all.
var ErrNoPeerAddress = errors.New("PEER_UNREACHABLE")

// --- Shares handlers ---

// handleListShares returns all active web shares.
func (s *Server) handleListShares(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ShareManager == nil {
		writeError(w, http.StatusServiceUnavailable, "share manager not available")
		return
	}
	shares := s.cfg.ShareManager.ListShares()
	writeJSON(w, http.StatusOK, map[string]any{"shares": shares})
}

// handleListShareHistory returns historical share records from SQLite.
func (s *Server) handleListShareHistory(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	opts := store.DefaultListOptions()
	if q := r.URL.Query().Get("q"); q != "" {
		opts.Search = q
	}
	if sb := r.URL.Query().Get("sort_by"); sb != "" {
		switch sb {
		case "time":
			opts.SortBy = store.SortByTime
		case "size":
			opts.SortBy = store.SortBySize
		case "name":
			opts.SortBy = store.SortByName
		case "status":
			opts.SortBy = store.SortByStatus
		}
	}
	if o := r.URL.Query().Get("order"); o == "asc" {
		opts.Order = store.SortAsc
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if _, err := parseInt(l, &n); err == nil && n > 0 {
			opts.Limit = n
		}
	}
	list, err := s.cfg.DB.ListShares(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list shares: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": list})
}

// handleExportShares streams all share records as CSV.
func (s *Server) handleExportShares(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=shares.csv")
	if err := s.cfg.DB.ExportSharesCSV(w); err != nil {
		slog.Error("export shares csv", "err", err)
	}
}

// handleExportTransfers streams all transfer records as CSV.
func (s *Server) handleExportTransfers(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=transfers.csv")
	if err := s.cfg.DB.ExportTransfersCSV(w); err != nil {
		slog.Error("export transfers csv", "err", err)
	}
}

// handleCreateShare creates a new web share.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ShareManager == nil {
		writeError(w, http.StatusServiceUnavailable, "share manager not available")
		return
	}

	var req struct {
		Path         string `json:"path"`
		Password     string `json:"password"`
		Expiry       int    `json:"expiry_seconds"` // seconds, 0 = default
		MaxDownloads int    `json:"max_downloads"`  // 0 = default
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	// Resolve path info
	info, err := os.Stat(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "path not accessible: "+err.Error())
		return
	}
	isDir := info.IsDir()
	name := filepath.Base(req.Path)

	// Count files and size
	var size int64
	var fileCount int
	if isDir {
		fileCount, size, err = share.CountFiles(req.Path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "count files: "+err.Error())
			return
		}
	} else {
		size = info.Size()
		fileCount = 1
	}

	// Defaults
	expiry := time.Duration(req.Expiry) * time.Second
	if expiry <= 0 {
		expiry = share.DefaultExpiry
	}
	maxDl := req.MaxDownloads
	if maxDl <= 0 {
		maxDl = share.MaxDownloadCount
	}

	sh, err := s.cfg.ShareManager.CreateShare(req.Path, isDir, name, size, req.Password, expiry, maxDl)
	if err != nil {
		if err == share.ErrShareLimit {
			writeError(w, http.StatusConflict, "concurrent share limit reached")
			return
		}
		writeError(w, http.StatusInternalServerError, "create share: "+err.Error())
		return
	}

	// Log to DB for historical records (non-fatal on failure).
	if s.cfg.DB != nil {
		expiryTime := time.Now().Add(expiry)
		maxDownloads := maxDl
		_ = s.cfg.DB.CreateShare(&store.ShareRecord{
			ID:           sh.Token,
			Kind:         "link",
			FilePath:     req.Path,
			Size:         size,
			Status:       "active",
			CreatedAt:    time.Now(),
			ExpiresAt:    &expiryTime,
			MaxDownloads: &maxDownloads,
		})
	}

	// Build download URL. The hostname comes from the request's Host header
	// (so LAN peers get the address they actually reached), but the port must
	// be the share server's port, not the API server's.
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		host = "localhost"
	}
	url := fmt.Sprintf("http://%s/dl/%s", net.JoinHostPort(host, fmt.Sprint(s.cfg.SharePort)), sh.Token)

	writeJSON(w, http.StatusCreated, map[string]any{
		"share":      sh,
		"url":        url,
		"file_count": fileCount,
		"total_size": size,
	})
}

// handleGetShare returns a single share by token.
func (s *Server) handleGetShare(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ShareManager == nil {
		writeError(w, http.StatusServiceUnavailable, "share manager not available")
		return
	}
	token := chi.URLParam(r, "id")
	if err := share.ValidateToken(token); err != nil {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	sh, err := s.cfg.ShareManager.GetShare(token, share.ClientIP(r.RemoteAddr))
	if err != nil {
		writeError(w, http.StatusNotFound, "share not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"share": sh})
}

// handleStopShare stops a share by token.
func (s *Server) handleStopShare(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ShareManager == nil {
		writeError(w, http.StatusServiceUnavailable, "share manager not available")
		return
	}
	token := chi.URLParam(r, "id")
	if !s.cfg.ShareManager.StopShare(token) {
		writeError(w, http.StatusNotFound, "share not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stopped": true})
}

// --- Transfers handlers ---

// handleListTransfers returns transfer history.
func (s *Server) handleListTransfers(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	opts := store.DefaultListOptions()
	if q := r.URL.Query().Get("q"); q != "" {
		opts.Search = q
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		var n int
		if _, err := parseInt(l, &n); err == nil && n > 0 {
			opts.Limit = n
		}
	}
	list, err := s.cfg.DB.ListTransfers(opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list transfers: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfers": list})
}

// handleGetTransfer returns a single transfer.
func (s *Server) handleGetTransfer(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	id := chi.URLParam(r, "id")
	t, err := s.cfg.DB.GetTransfer(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "transfer not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfer": t})
}

// handleDeleteTransfer deletes a transfer record.
func (s *Server) handleDeleteTransfer(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DB == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.cfg.DB.DeleteTransfer(id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete transfer: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// handleCreateTransfer initiates an outgoing file transfer.
//
// Request body: {"peer_id":"<device-id>","file_path":"/path/to/file"}
//
// The handler creates the transfer record immediately (so the UI gets an ID
// to poll), then runs SendFileUseCase in a background goroutine. The usecase
// dials the peer, performs the Noise XX handshake, sends the file header,
// and streams the file body, updating transfer status/progress along the way.
func (s *Server) handleCreateTransfer(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TransferMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "transfer manager not available")
		return
	}
	if s.cfg.Discovery == nil {
		writeError(w, http.StatusServiceUnavailable, "discovery not available")
		return
	}

	var req struct {
		PeerID   string `json:"peer_id"`
		FilePath string `json:"file_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.PeerID == "" {
		writeError(w, http.StatusBadRequest, "peer_id is required")
		return
	}
	if req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "file_path is required")
		return
	}

	// Resolve file info.
	info, err := os.Stat(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file not accessible: "+err.Error())
		return
	}
	if info.IsDir() {
		writeError(w, http.StatusBadRequest, "directories not yet supported")
		return
	}

	// Look up peer from discovery to get its address.
	peer := s.findDeviceByID(req.PeerID)
	if peer == nil {
		writeError(w, http.StatusNotFound, "peer not found or offline")
		return
	}
	sel, err := peerAddress(peer)
	if err != nil {
		code := "PEER_UNREACHABLE"
		if errors.Is(err, ErrIncompatibleIPVersion) {
			code = "INCOMPATIBLE_IP_VERSION"
		}
		writeErrorCode(w, http.StatusServiceUnavailable, code,
			"peer has no reachable address: "+err.Error())
		return
	}
	peerAddr := sel.DialAddr

	// Create the transfer record now so we can return the ID immediately.
	t, err := s.cfg.TransferMgr.Create(req.PeerID, peer.Name, req.FilePath, info.Name(), info.Size())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create transfer: "+err.Error())
		return
	}

	// Run the send usecase in the background. The usecase picks up the
	// pre-created transfer via TransferID and handles all status transitions.
	//
	// IMPORTANT: r.Context() is canceled the instant this handler returns
	// (after writeJSON), which would abort an async transfer before it even
	// dialed. Use the application-scoped context stored on the Server; fall
	// back to context.Background() if Serve() was never called (e.g. tests).
	appCtx := s.appCtx
	if appCtx == nil {
		appCtx = context.Background()
	}
	uc := usecase.NewSendFileUseCase(s.cfg.TransferMgr)
	go func() {
		err := uc.Execute(appCtx, usecase.SendConfig{
			PeerID:     req.PeerID,
			PeerAddr:   peerAddr,
			PeerName:   peer.Name,
			FilePath:   req.FilePath,
			StaticKeys: s.cfg.StaticKeys,
			TransferID: t.ID,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "gcd: send transfer %s: %v\n", t.ID, err)
		}
	}()

	writeJSON(w, http.StatusCreated, map[string]any{"transfer": t})
}

// findDeviceByID searches the discovery device list for a peer with the
// given ID. Returns nil if not found.
func (s *Server) findDeviceByID(id string) *discovery.Device {
	if s.cfg.Discovery == nil {
		return nil
	}
	for _, d := range s.cfg.Discovery.Devices() {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// peerAddress picks the dial address for a peer using RFC 6724 destination
// selection against the local interface source IPs. Returns:
//
//   - d.DialAddr and a non-empty Version ("4"/"6") on success.
//   - err = ErrNoPeerAddress when the peer advertises no IP family candidates.
//   - err = ErrIncompatibleIPVersion when both advertise IPs but none has a
//     matching local source (e.g. v6-only peer + v4-only local).
//
// The Version field records the selected destination's IP family so the
// caller can surface diagnostics (which stack was used).
type peerSelectResult struct {
	DialAddr string
	Version  string
}

func peerAddress(d *discovery.Device) (peerSelectResult, error) {
	return peerAddressWith(d, discovery.LocalSourceIPs())
}

// peerAddressWith is the testable variant that takes explicit source IPs.
func peerAddressWith(d *discovery.Device, sources []string) (peerSelectResult, error) {
	if d.Port <= 0 {
		return peerSelectResult{}, ErrNoPeerAddress
	}
	dsts := make([]string, 0, len(d.IPv4)+len(d.IPv6))
	dsts = append(dsts, d.IPv4...)
	dsts = append(dsts, d.IPv6...)
	if len(dsts) == 0 {
		return peerSelectResult{}, ErrNoPeerAddress
	}
	pairs := lanosnet.SelectAddresses(dsts, sources, d.Port)
	if len(pairs) == 0 {
		return peerSelectResult{}, ErrIncompatibleIPVersion
	}
	ver := "4"
	if pairs[0].IsV6 {
		ver = "6"
	}
	return peerSelectResult{DialAddr: pairs[0].Destination, Version: ver}, nil
}

// --- Incoming (receive) handlers ---

// handleListIncoming returns all active incoming transfer prompts.
func (s *Server) handleListIncoming(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReceiveMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "receive manager not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": s.cfg.ReceiveMgr.List()})
}

// handleAcceptIncoming accepts an incoming transfer.
func (s *Server) handleAcceptIncoming(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReceiveMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "receive manager not available")
		return
	}
	id := chi.URLParam(r, "id")
	var req struct {
		SavePath string `json:"save_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.SavePath == "" {
		writeError(w, http.StatusBadRequest, "save_path is required")
		return
	}
	inc, err := s.cfg.ReceiveMgr.Accept(id, req.SavePath)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": inc})
}

// handleRejectIncoming rejects an incoming transfer.
func (s *Server) handleRejectIncoming(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReceiveMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "receive manager not available")
		return
	}
	id := chi.URLParam(r, "id")
	inc, err := s.cfg.ReceiveMgr.Reject(id)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": inc})
}

// handleCancelIncoming cancels an incoming transfer.
func (s *Server) handleCancelIncoming(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReceiveMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "receive manager not available")
		return
	}
	id := chi.URLParam(r, "id")
	inc, err := s.cfg.ReceiveMgr.Cancel(id)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incoming": inc})
}

// handleCancelTransfer cancels an outgoing transfer.
func (s *Server) handleCancelTransfer(w http.ResponseWriter, r *http.Request) {
	if s.cfg.TransferMgr == nil {
		writeError(w, http.StatusServiceUnavailable, "transfer manager not available")
		return
	}
	id := chi.URLParam(r, "id")
	t, err := s.cfg.TransferMgr.Cancel(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"transfer": t})
}

// --- Settings handlers ---

// handleGetSettings returns the current configuration.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		writeError(w, http.StatusServiceUnavailable, "config not available")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": s.cfg.Config})
}

// handleUpdateSettings updates the configuration.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Config == nil {
		writeError(w, http.StatusServiceUnavailable, "config not available")
		return
	}
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := s.cfg.Config.Apply(updates); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// --- Helpers ---

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    http.StatusText(status),
			"message": msg,
		},
	})
}

// writeErrorCode emits a structured error response with a custom machine code
// in addition to the HTTP status. Used where the contract calls for a stable
// error code distinct from HTTP StatusText (e.g. INCOMPATIBLE_IP_VERSION).
func writeErrorCode(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}

func parseInt(s string, out *int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	*out = n
	return n, nil
}
