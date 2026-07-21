package store

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// ShareRecord represents a row in the shares table.
type ShareRecord struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`      // "link" or "direct"
	Target       string    `json:"target"`    // device name or URL
	FilePath     string    `json:"file_path"`
	Size         int64     `json:"size"`
	Status       string    `json:"status"`    // active, expired, stopped, completed
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Downloads    int       `json:"downloads"`
	MaxDownloads *int      `json:"max_downloads,omitempty"`
}

// TransferRecord represents a row in the transfers table.
type TransferRecord struct {
	ID           string     `json:"id"`
	Direction    string     `json:"direction"` // "send" or "recv"
	PeerDeviceID string     `json:"peer_device_id"`
	PeerName     string     `json:"peer_name"`
	FilePath     string     `json:"file_path"`
	Size         int64      `json:"size"`
	Status       string     `json:"status"` // pending, active, completed, failed, cancelled
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// SortField specifies the column to sort by.
type SortField string

const (
	SortByTime   SortField = "time"
	SortBySize   SortField = "size"
	SortByName   SortField = "name"
	SortByStatus SortField = "status"
)

// SortOrder specifies ascending or descending.
type SortOrder string

const (
	SortDesc SortOrder = "DESC"
	SortAsc  SortOrder = "ASC"
)

// ListOptions controls pagination, search, and sorting for list queries.
type ListOptions struct {
	Search  string    // fuzzy match on file_path
	SortBy  SortField
	Order   SortOrder
	Limit   int
	Offset  int
}

// DefaultListOptions returns sensible defaults.
func DefaultListOptions() ListOptions {
	return ListOptions{
		SortBy: SortByTime,
		Order:  SortDesc,
		Limit:  100,
	}
}

// --- Shares CRUD ---

// CreateShare inserts a new share record.
func (db *DB) CreateShare(r *ShareRecord) error {
	_, err := db.Exec(
		`INSERT INTO shares (id, kind, target, file_path, size, status, created_at, expires_at, downloads, max_downloads)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Kind, r.Target, r.FilePath, r.Size, r.Status,
		r.CreatedAt.Unix(), nullTime(r.ExpiresAt), r.Downloads, nullInt(r.MaxDownloads),
	)
	return err
}

// GetShare retrieves a share by ID.
func (db *DB) GetShare(id string) (*ShareRecord, error) {
	var r ShareRecord
	var expiresAt, maxDownloads sql.NullInt64
	var target sql.NullString
	var createdAt int64
	err := db.QueryRow(
		`SELECT id, kind, target, file_path, size, status, created_at, expires_at, downloads, max_downloads
		 FROM shares WHERE id = ?`, id,
	).Scan(&r.ID, &r.Kind, &target, &r.FilePath, &r.Size, &r.Status,
		&createdAt, &expiresAt, &r.Downloads, &maxDownloads)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = time.Unix(createdAt, 0)
	r.Target = target.String
	if expiresAt.Valid {
		t := time.Unix(expiresAt.Int64, 0)
		r.ExpiresAt = &t
	}
	if maxDownloads.Valid {
		v := int(maxDownloads.Int64)
		r.MaxDownloads = &v
	}
	return &r, nil
}

// UpdateShareStatus updates the status of a share.
func (db *DB) UpdateShareStatus(id, status string) error {
	_, err := db.Exec(`UPDATE shares SET status = ? WHERE id = ?`, status, id)
	return err
}

// IncrementShareDownloads increments the download counter.
func (db *DB) IncrementShareDownloads(id string) error {
	_, err := db.Exec(`UPDATE shares SET downloads = downloads + 1 WHERE id = ?`, id)
	return err
}

// DeleteShare removes a share record.
func (db *DB) DeleteShare(id string) error {
	_, err := db.Exec(`DELETE FROM shares WHERE id = ?`, id)
	return err
}

// ListShares returns shares matching the given options.
func (db *DB) ListShares(opts ListOptions) ([]*ShareRecord, error) {
	orderBy := "created_at"
	switch opts.SortBy {
	case SortBySize:
		orderBy = "size"
	case SortByName:
		orderBy = "file_path"
	case SortByStatus:
		orderBy = "status"
	}
	order := "DESC"
	if opts.Order == SortAsc {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, kind, target, file_path, size, status, created_at, expires_at, downloads, max_downloads
		 FROM shares WHERE file_path LIKE ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		orderBy, order,
	)
	search := "%" + opts.Search + "%"
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.Query(query, search, limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ShareRecord
	for rows.Next() {
		var r ShareRecord
		var expiresAt, maxDownloads sql.NullInt64
		var target sql.NullString
		var createdAt int64
		if err := rows.Scan(&r.ID, &r.Kind, &target, &r.FilePath, &r.Size, &r.Status,
			&createdAt, &expiresAt, &r.Downloads, &maxDownloads); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(createdAt, 0)
		r.Target = target.String
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			r.ExpiresAt = &t
		}
		if maxDownloads.Valid {
			v := int(maxDownloads.Int64)
			r.MaxDownloads = &v
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// CountShares returns the total number of share records.
func (db *DB) CountShares() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM shares`).Scan(&n)
	return n, err
}

// --- Transfers CRUD ---

// CreateTransfer inserts a new transfer record.
func (db *DB) CreateTransfer(r *TransferRecord) error {
	_, err := db.Exec(
		`INSERT INTO transfers (id, direction, peer_device_id, peer_name, file_path, size, status, started_at, finished_at, error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Direction, r.PeerDeviceID, r.PeerName, r.FilePath, r.Size, r.Status,
		r.StartedAt.Unix(), nullTime(r.FinishedAt), r.Error,
	)
	return err
}

// GetTransfer retrieves a transfer by ID.
func (db *DB) GetTransfer(id string) (*TransferRecord, error) {
	var r TransferRecord
	var finishedAt sql.NullInt64
	var peerName, errStr sql.NullString
	var startedAt int64
	err := db.QueryRow(
		`SELECT id, direction, peer_device_id, peer_name, file_path, size, status, started_at, finished_at, error
		 FROM transfers WHERE id = ?`, id,
	).Scan(&r.ID, &r.Direction, &r.PeerDeviceID, &peerName, &r.FilePath, &r.Size, &r.Status,
		&startedAt, &finishedAt, &errStr)
	if err != nil {
		return nil, err
	}
	r.StartedAt = time.Unix(startedAt, 0)
	r.PeerName = peerName.String
	r.Error = errStr.String
	if finishedAt.Valid {
		t := time.Unix(finishedAt.Int64, 0)
		r.FinishedAt = &t
	}
	return &r, nil
}

// UpdateTransferStatus updates the status and optionally the finished time/error.
func (db *DB) UpdateTransferStatus(id, status string, finishedAt *time.Time, errMsg string) error {
	_, err := db.Exec(
		`UPDATE transfers SET status = ?, finished_at = ?, error = ? WHERE id = ?`,
		status, nullTime(finishedAt), errMsg, id,
	)
	return err
}

// DeleteTransfer removes a transfer record.
func (db *DB) DeleteTransfer(id string) error {
	_, err := db.Exec(`DELETE FROM transfers WHERE id = ?`, id)
	return err
}

// ListTransfers returns transfers matching the given options.
func (db *DB) ListTransfers(opts ListOptions) ([]*TransferRecord, error) {
	orderBy := "started_at"
	switch opts.SortBy {
	case SortBySize:
		orderBy = "size"
	case SortByName:
		orderBy = "file_path"
	case SortByStatus:
		orderBy = "status"
	}
	order := "DESC"
	if opts.Order == SortAsc {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, direction, peer_device_id, peer_name, file_path, size, status, started_at, finished_at, error
		 FROM transfers WHERE file_path LIKE ? ORDER BY %s %s LIMIT ? OFFSET ?`,
		orderBy, order,
	)
	search := "%" + opts.Search + "%"
	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}

	rows, err := db.Query(query, search, limit, opts.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*TransferRecord
	for rows.Next() {
		var r TransferRecord
		var finishedAt sql.NullInt64
		var peerName, errStr sql.NullString
		var startedAt int64
		if err := rows.Scan(&r.ID, &r.Direction, &r.PeerDeviceID, &peerName, &r.FilePath, &r.Size, &r.Status,
			&startedAt, &finishedAt, &errStr); err != nil {
			return nil, err
		}
		r.StartedAt = time.Unix(startedAt, 0)
		r.PeerName = peerName.String
		r.Error = errStr.String
		if finishedAt.Valid {
			t := time.Unix(finishedAt.Int64, 0)
			r.FinishedAt = &t
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// CountTransfers returns the total number of transfer records.
func (db *DB) CountTransfers() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM transfers`).Scan(&n)
	return n, err
}

// --- Batch operations ---

// DeleteShares removes multiple shares by ID.
func (db *DB) DeleteShares(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`DELETE FROM shares WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteTransfers removes multiple transfers by ID.
func (db *DB) DeleteTransfers(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`DELETE FROM transfers WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- Retention policy ---

// EnforceRetention deletes oldest records beyond the keep limit.
// keep = 0 means keep all (no deletion).
func (db *DB) EnforceRetention(keep int) error {
	if keep <= 0 {
		return nil
	}
	// Delete oldest shares
	_, err := db.Exec(
		`DELETE FROM shares WHERE id NOT IN (SELECT id FROM shares ORDER BY created_at DESC LIMIT ?)`, keep,
	)
	if err != nil {
		return err
	}
	// Delete oldest transfers
	_, err = db.Exec(
		`DELETE FROM transfers WHERE id NOT IN (SELECT id FROM transfers ORDER BY started_at DESC LIMIT ?)`, keep,
	)
	return err
}

// --- CSV export ---

// ExportSharesCSV writes all shares to w in CSV format.
func (db *DB) ExportSharesCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"id", "kind", "target", "file_path", "size", "status", "created_at", "expires_at", "downloads", "max_downloads"}); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT id, kind, target, file_path, size, status, created_at, expires_at, downloads, max_downloads FROM shares ORDER BY created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind, target, path, status string
		var size, downloads int64
		var createdAt int64
		var expiresAt, maxDownloads sql.NullInt64
		if err := rows.Scan(&id, &kind, &target, &path, &size, &status, &createdAt, &expiresAt, &downloads, &maxDownloads); err != nil {
			return err
		}
		row := []string{
			id, kind, target, path, fmt.Sprintf("%d", size), status,
			time.Unix(createdAt, 0).Format(time.RFC3339),
			nullTimeStr(expiresAt), fmt.Sprintf("%d", downloads), nullIntStr(maxDownloads),
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ExportTransfersCSV writes all transfers to w in CSV format.
func (db *DB) ExportTransfersCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"id", "direction", "peer_device_id", "peer_name", "file_path", "size", "status", "started_at", "finished_at", "error"}); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT id, direction, peer_device_id, peer_name, file_path, size, status, started_at, finished_at, error FROM transfers ORDER BY started_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, dir, pid, pname, path, status, errStr string
		var size int64
		var startedAt int64
		var finishedAt sql.NullInt64
		if err := rows.Scan(&id, &dir, &pid, &pname, &path, &size, &status, &startedAt, &finishedAt, &errStr); err != nil {
			return err
		}
		row := []string{
			id, dir, pid, pname, path, fmt.Sprintf("%d", size), status,
			time.Unix(startedAt, 0).Format(time.RFC3339),
			nullTimeStr(finishedAt), errStr,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// --- Helpers ---

func nullTime(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.Unix(), Valid: true}
}

func nullInt(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*i), Valid: true}
}

func nullTimeStr(n sql.NullInt64) string {
	if !n.Valid {
		return ""
	}
	return time.Unix(n.Int64, 0).Format(time.RFC3339)
}

func nullIntStr(n sql.NullInt64) string {
	if !n.Valid {
		return ""
	}
	return fmt.Sprintf("%d", n.Int64)
}
