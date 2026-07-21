package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lanos/lanos/core/share"
	"github.com/lanos/lanos/core/store"
)

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

// handleCreateShare creates a new web share.
func (s *Server) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ShareManager == nil {
		writeError(w, http.StatusServiceUnavailable, "share manager not available")
		return
	}

	var req struct {
		Path         string `json:"path"`
		Password     string `json:"password"`
		Expiry       int    `json:"expiry_seconds"`  // seconds, 0 = default
		MaxDownloads int    `json:"max_downloads"`   // 0 = default
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

	// Build response with download URL
	url := ""
	if s.cfg.Config != nil {
		url = share.ShareURL("localhost", 52103, sh.Token)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"share":       sh,
		"url":         url,
		"file_count":  fileCount,
		"total_size":  size,
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
	// TODO: apply updates to config and persist
	writeJSON(w, http.StatusOK, map[string]any{"updated": true, "updates": updates})
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

func parseInt(s string, out *int) (int, error) {
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		return 0, err
	}
	*out = n
	return n, nil
}
