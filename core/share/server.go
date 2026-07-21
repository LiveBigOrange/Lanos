package share

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server is the HTTP server that serves web shares to browsers.
type Server struct {
	manager  *Manager
	listener net.Listener
	port     int
}

// NewServer creates a share HTTP server bound to the given listener.
// The listener is typically created by core/net on a random port in
// the 52100-52999 range.
func NewServer(manager *Manager, listener net.Listener) *Server {
	return &Server{
		manager:  manager,
		listener: listener,
		port:     listener.Addr().(*net.TCPAddr).Port,
	}
}

// Port returns the bound port.
func (s *Server) Port() int { return s.port }

// Start begins serving HTTP. It blocks until the listener is closed.
func (s *Server) Start() error {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(ZipStreamTimeout))

	// Download routes
	r.Get("/dl/{token}", s.handleDownloadPage)
	r.Post("/dl/{token}", s.handleDownload)
	r.Get("/dl/{token}/raw", s.handleDownload) // direct download without password page

	// Status API
	r.Get("/api/share/{token}/status", s.handleStatus)

	// QR code placeholder (Flutter generates QR client-side; this endpoint
	// returns the share URL for convenience)
	r.Get("/qr/{token}", s.handleQR)

	slog.Info("share server started", "port", s.port)
	return http.Serve(s.listener, r)
}

// Close shuts down the server and stops all shares.
func (s *Server) Close() error {
	s.manager.StopAll()
	return s.listener.Close()
}

// --- Handlers ---

// handleDownloadPage serves the minimal HTML download page.
func (s *Server) handleDownloadPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	ip := ClientIP(r.RemoteAddr)

	if err := ValidateToken(token); err != nil {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	share, err := s.manager.GetShare(token, ip)
	if err != nil {
		if err == ErrIPBanned {
			http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "share not found or expired", http.StatusNotFound)
		return
	}

	// If no password required and ?dl=1, start download directly
	if !share.HasPassword && r.URL.Query().Get("dl") == "1" {
		s.serveDownload(w, r, share)
		return
	}

	s.renderDownloadPage(w, share, "")
}

// handleDownload processes the download form (with optional password).
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	ip := ClientIP(r.RemoteAddr)

	if err := ValidateToken(token); err != nil {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	share, err := s.manager.GetShare(token, ip)
	if err != nil {
		if err == ErrIPBanned {
			http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "share not found or expired", http.StatusNotFound)
		return
	}

	// Check password if required
	if share.HasPassword {
		password := r.FormValue("password")
		if err := s.manager.CheckPassword(share, password, ip); err != nil {
			if err == ErrIPBanned {
				http.Error(w, "too many attempts, try again later", http.StatusTooManyRequests)
				return
			}
			s.renderDownloadPage(w, share, "Incorrect password")
			return
		}
	}

	s.serveDownload(w, r, share)
}

// serveDownload streams the file or ZIP archive to the client.
func (s *Server) serveDownload(w http.ResponseWriter, r *http.Request, share *Share) {
	// Record download before serving (prevents race on count limit)
	s.manager.RecordDownload(share.Token)

	if share.IsDir {
		s.serveZip(w, r, share)
	} else {
		s.serveFile(w, r, share)
	}
}

// serveFile streams a single file.
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, share *Share) {
	file, err := os.Open(share.Path)
	if err != nil {
		slog.Error("share file open failed", "path", share.Path, "error", err)
		http.Error(w, "file unavailable", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", urlEncode(share.Name)))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", share.Size))

	slog.Info("share download started", "token", share.Token[:8]+"...", "name", share.Name, "size", share.Size)
	http.ServeContent(w, r, share.Name, time.Time{}, file)
}

// serveZip streams a directory as a ZIP archive.
func (s *Server) serveZip(w http.ResponseWriter, r *http.Request, share *Share) {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s.zip", urlEncode(share.Name)))
	w.Header().Set("Content-Type", "application/zip")

	slog.Info("share zip started", "token", share.Token[:8]+"...", "name", share.Name)

	// Use io.Pipe for streaming: zip writer -> pipe -> http response
	pr, pw := io.Pipe()
	defer pr.Close()

	go func() {
		defer pw.Close()
		err := StreamZip(pw, share.Path, func(path string, size int64) {
			slog.Debug("zip add", "path", path, "size", size)
		})
		if err != nil {
			slog.Error("zip stream failed", "token", share.Token[:8]+"...", "error", err)
			pw.CloseWithError(err)
		}
	}()

	_, err := io.Copy(w, pr)
	if err != nil {
		slog.Error("zip download failed", "token", share.Token[:8]+"...", "error", err)
	}
}

// handleStatus returns JSON status for a share.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	ip := ClientIP(r.RemoteAddr)

	if err := ValidateToken(token); err != nil {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	share, err := s.manager.GetShare(token, ip)
	if err != nil {
		if err == ErrIPBanned {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":              share.Token,
		"name":               share.Name,
		"size":               share.Size,
		"is_dir":             share.IsDir,
		"has_password":       share.HasPassword,
		"downloads":          share.Downloads,
		"max_downloads":      share.MaxDownloads,
		"remaining_downloads": share.RemainingDownloads(),
		"remaining_seconds":  int(share.RemainingTime().Seconds()),
		"created_at":         share.CreatedAt.Format(time.RFC3339),
	})
}

// handleQR returns the share URL as plain text (for QR generation).
func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if err := ValidateToken(token); err != nil {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	share, err := s.manager.GetShare(token, ClientIP(r.RemoteAddr))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = fmt.Sprintf("localhost:%d", s.port)
	}

	url := fmt.Sprintf("%s://%s/dl/%s", scheme, host, share.Token)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(url))
}

// --- HTML template ---

var downloadPageTmpl = template.Must(template.New("dl").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Download - {{.Name}}</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;max-width:480px;margin:2rem auto;padding:0 1rem;color:#333}
.card{border:1px solid #ddd;border-radius:12px;padding:2rem;box-shadow:0 2px 8px rgba(0,0,0,.08)}
h1{font-size:1.25rem;margin:0 0 .5rem}
.meta{color:#666;font-size:.875rem;margin-bottom:1.5rem}
.btn{display:inline-block;background:#007aff;color:#fff;padding:.75rem 1.5rem;border-radius:8px;text-decoration:none;font-weight:600;border:none;cursor:pointer;font-size:1rem}
.btn:hover{background:#0056cc}
input[type=password]{width:100%;padding:.75rem;border:1px solid #ccc;border-radius:8px;margin-bottom:1rem;font-size:1rem;box-sizing:border-box}
.error{color:#d32f2f;font-size:.875rem;margin-bottom:1rem}
.info{color:#666;font-size:.75rem;margin-top:1rem}
</style>
</head>
<body>
<div class="card">
<h1>{{.Name}}</h1>
<div class="meta">
{{if .IsDir}}ZIP archive (folder){{else}}File{{end}} &middot; {{.SizeHuman}}
{{if .HasPassword}} &middot; Password required{{end}}
</div>
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
{{if .HasPassword}}
<form method="POST" action="/dl/{{.Token}}">
<input type="password" name="password" placeholder="Enter password" required autofocus>
<button type="submit" class="btn">Download</button>
</form>
{{else}}
<a class="btn" href="/dl/{{.Token}}?dl=1">Download</a>
{{end}}
<div class="info">
{{if .MaxDownloads}}Downloads remaining: {{.RemainingDownloads}}<br>{{end}}
Expires in: {{.RemainingTime}}
</div>
</div>
</body>
</html>`))

type downloadPageData struct {
	Token              string
	Name               string
	IsDir              bool
	SizeHuman          string
	HasPassword        bool
	Error              string
	MaxDownloads       int
	RemainingDownloads int
	RemainingTime      string
}

func (s *Server) renderDownloadPage(w http.ResponseWriter, share *Share, errMsg string) {
	data := downloadPageData{
		Token:              share.Token,
		Name:               share.Name,
		IsDir:              share.IsDir,
		SizeHuman:          humanizeSize(share.Size),
		HasPassword:        share.HasPassword,
		Error:              errMsg,
		MaxDownloads:       share.MaxDownloads,
		RemainingDownloads: share.RemainingDownloads(),
		RemainingTime:      formatDuration(share.RemainingTime()),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	downloadPageTmpl.Execute(w, data)
}

// --- Helpers ---

func humanizeSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func urlEncode(s string) string {
	// Minimal URL encoding for Content-Disposition filename*
	t := template.URLQueryEscaper(s)
	return t
}

// ShareURL builds the full download URL for a share on the given host.
func ShareURL(host string, port int, token string) string {
	return fmt.Sprintf("http://%s:%d/dl/%s", host, port, token)
}

// ShareURLForAddr builds the download URL using the server's bound address.
func (s *Server) ShareURLForAddr(token string) string {
	addr := s.listener.Addr().(*net.TCPAddr)
	host := addr.IP.String()
	if addr.IP.IsUnspecified() {
		host = "localhost"
	}
	// IPv6 bracket wrapping
	if addr.IP.To4() == nil {
		host = "[" + host + "]"
	}
	return ShareURL(host, s.port, token)
}

// --- Utility ---

// FileName returns the base name of a path, suitable for display.
func FileName(path string) string {
	return filepath.Base(path)
}
