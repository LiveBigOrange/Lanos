// Package transfer: file chunking with per-chunk SHA256 verification.
//
// Implements docs/PROTOCOL.md §4.3: files are split into fixed 4 MiB chunks
// (last chunk may be smaller), chunk_idx starts at 0, the receiver writes each
// chunk to transfer_cache/<task_id>/chunk_<idx> and updates
// transfer_meta/<task_id>.meta. The meta file records which chunks are
// ACKed/received so a resumed transfer can skip already-verified chunks.
//
// Per-chunk SHA256 hashes are carried in a Manifest that the sender transmits
// up front (in the META frame payload). This is a transfer-layer extension to
// the protocol's META JSON (§4.2 lists task_id/file_path/size/relative_path);
// the manifest is added so the receiver can verify each chunk independently
// without a second round trip.
package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ChunkSize is the fixed chunk size: 4 MiB (docs/PROTOCOL.md §4.3).
const ChunkSize = 4 << 20

// MaxChunkSize is the largest chunk this layer will handle (sanity bound for
// inbound frames). Equal to transport.MaxPayloadLen minus frame overhead.
const MaxChunkSize = ChunkSize

var (
	ErrChunkIndex       = errors.New("transfer: chunk index out of range")
	ErrChunkHash        = errors.New("transfer: chunk sha256 mismatch")
	ErrChunkSizeWrong   = errors.New("transfer: chunk size mismatch")
	ErrIncomplete       = errors.New("transfer: incomplete chunk set")
	ErrManifestMismatch = errors.New("transfer: manifest does not match received chunks")
)

// Manifest describes the chunked layout of one file transfer. The sender builds
// it before sending and transmits it (serialized) in the META frame so the
// receiver can verify each chunk's SHA256 as it arrives.
type Manifest struct {
	TaskID      string   `json:"task_id"`
	FileName    string   `json:"file_name"`
	TotalSize   int64    `json:"total_size"`
	ChunkCount  int      `json:"chunk_count"`
	ChunkHashes []string `json:"chunk_hashes"` // hex sha256 per chunk, len == ChunkCount
}

// ChunkCount returns the number of 4 MiB chunks needed for size bytes.
func ChunkCount(size int64) int {
	if size <= 0 {
		return 0
	}
	return int((size + ChunkSize - 1) / ChunkSize)
}

// ChunkHashHex computes the hex-encoded SHA256 of data.
func ChunkHashHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- Sender side: ChunkReader ---

// ChunkReader streams a file as 4 MiB chunks, computing each chunk's SHA256.
// It is safe for single-goroutine sequential use.
type ChunkReader struct {
	f         *os.File
	idx       int
	remaining int64
	total     int64
	buf       []byte
}

// ChunkResult is one chunk yielded by ChunkReader.
type ChunkResult struct {
	Idx    int
	Data   []byte
	Sha256 string // hex
}

// NewChunkReader opens path for reading in chunks.
func NewChunkReader(path string) (*ChunkReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("transfer: open %s: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("transfer: stat %s: %w", path, err)
	}
	return &ChunkReader{
		f:         f,
		total:     info.Size(),
		remaining: info.Size(),
		buf:       make([]byte, ChunkSize),
	}, nil
}

// TotalSize returns the file size in bytes.
func (r *ChunkReader) TotalSize() int64 { return r.total }

// ChunkCount returns the total number of chunks in the file.
func (r *ChunkReader) ChunkCount() int { return ChunkCount(r.total) }

// Next reads and returns the next chunk, or io.EOF when done.
func (r *ChunkReader) Next() (*ChunkResult, error) {
	if r.remaining <= 0 {
		return nil, io.EOF
	}
	n := int64(ChunkSize)
	if n > r.remaining {
		n = r.remaining
	}
	if _, err := io.ReadFull(r.f, r.buf[:n]); err != nil {
		return nil, fmt.Errorf("transfer: read chunk %d: %w", r.idx, err)
	}
	data := make([]byte, n)
	copy(data, r.buf[:n])
	res := &ChunkResult{
		Idx:    r.idx,
		Data:   data,
		Sha256: ChunkHashHex(data),
	}
	r.idx++
	r.remaining -= n
	return res, nil
}

// BuildManifest reads the entire file's worth of chunk hashes without holding
// the chunk data. Used by the sender to build the META manifest up front.
func (r *ChunkReader) BuildManifest(taskID, fileName string) (*Manifest, error) {
	hashes := make([]string, 0, r.ChunkCount())
	idx := 0
	for {
		n := int64(ChunkSize)
		if n > r.remaining {
			n = r.remaining
		}
		if n <= 0 {
			break
		}
		if _, err := io.ReadFull(r.f, r.buf[:n]); err != nil {
			return nil, fmt.Errorf("transfer: manifest chunk %d: %w", idx, err)
		}
		hashes = append(hashes, ChunkHashHex(r.buf[:n]))
		idx++
		r.remaining -= n
	}
	return &Manifest{
		TaskID:      taskID,
		FileName:    fileName,
		TotalSize:   r.total,
		ChunkCount:  len(hashes),
		ChunkHashes: hashes,
	}, nil
}

// Close releases the underlying file.
func (r *ChunkReader) Close() error { return r.f.Close() }

// --- Receiver side: ChunkWriter ---

// ChunkWriter receives chunks into a cache directory, verifies each against the
// manifest, persists received state to a .meta file for resume, and finally
// assembles all chunks into the destination file.
type ChunkWriter struct {
	mu        sync.Mutex
	cacheDir  string // <cacheRoot>/<taskID>
	metaPath  string // <cacheRoot>/<taskID>.meta
	manifest  Manifest
	received  []bool
	receivedN int
}

// chunkMetaFile is the persisted resume state.
type chunkMetaFile struct {
	Manifest Manifest `json:"manifest"`
	Received []bool   `json:"received"`
}

// NewChunkWriter prepares a cache directory for taskID and loads any existing
// resume state. manifest must be the sender-provided manifest (from META).
func NewChunkWriter(cacheRoot, taskID string, manifest Manifest) (*ChunkWriter, error) {
	cacheDir := filepath.Join(cacheRoot, taskID)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("transfer: mkdir cache: %w", err)
	}
	cw := &ChunkWriter{
		cacheDir: cacheDir,
		metaPath: filepath.Join(cacheRoot, taskID+".meta"),
		manifest: manifest,
		received: make([]bool, manifest.ChunkCount),
	}
	cw.loadMeta()
	return cw, nil
}

// loadMeta reads the resume state file if present.
func (cw *ChunkWriter) loadMeta() {
	b, err := os.ReadFile(cw.metaPath)
	if err != nil {
		return
	}
	var m chunkMetaFile
	if json.Unmarshal(b, &m) != nil {
		return
	}
	// Adopt received state only if it matches the current manifest.
	if m.Manifest.TaskID == cw.manifest.TaskID && len(m.Received) == len(cw.received) {
		cw.received = m.Received
		for _, r := range cw.received {
			if r {
				cw.receivedN++
			}
		}
	}
}

// saveMeta persists resume state atomically.
func (cw *ChunkWriter) saveMeta() error {
	m := chunkMetaFile{Manifest: cw.manifest, Received: cw.received}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := cw.metaPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, cw.metaPath)
}

// IsReceived reports whether chunk idx was already verified and cached.
func (cw *ChunkWriter) IsReceived(idx int) bool {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if idx < 0 || idx >= len(cw.received) {
		return false
	}
	return cw.received[idx]
}

// ReceivedCount returns the number of verified+cached chunks so far.
func (cw *ChunkWriter) ReceivedCount() int {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return cw.receivedN
}

// WriteChunk verifies and caches one chunk. It returns ErrChunkHash if the
// SHA256 does not match the manifest. Already-received chunks are no-ops.
func (cw *ChunkWriter) WriteChunk(idx int, data []byte) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	if idx < 0 || idx >= cw.manifest.ChunkCount {
		return ErrChunkIndex
	}
	if cw.received[idx] {
		return nil // already have it (resume / duplicate ACK)
	}
	if ChunkHashHex(data) != cw.manifest.ChunkHashes[idx] {
		return ErrChunkHash
	}
	// Validate chunk size: all but the last must be exactly ChunkSize.
	expected := int64(ChunkSize)
	if idx == cw.manifest.ChunkCount-1 {
		rem := cw.manifest.TotalSize - int64(idx)*ChunkSize
		if rem > 0 {
			expected = rem
		}
	}
	if int64(len(data)) != expected {
		return fmt.Errorf("%w: chunk %d got %d want %d", ErrChunkSizeWrong, idx, len(data), expected)
	}
	chunkPath := filepath.Join(cw.cacheDir, chunkFileName(idx))
	if err := os.WriteFile(chunkPath, data, 0o644); err != nil {
		return fmt.Errorf("transfer: write chunk %d: %w", idx, err)
	}
	cw.received[idx] = true
	cw.receivedN++
	return cw.saveMeta()
}

// Assemble concatenates all cached chunks into destPath and verifies the total
// size. Requires all chunks to be received. Returns ErrIncomplete otherwise.
func (cw *ChunkWriter) Assemble(destPath string) error {
	cw.mu.Lock()
	if cw.receivedN != cw.manifest.ChunkCount {
		cw.mu.Unlock()
		return fmt.Errorf("%w: %d/%d chunks", ErrIncomplete, cw.receivedN, cw.manifest.ChunkCount)
	}
	count := cw.manifest.ChunkCount
	cacheDir := cw.cacheDir
	total := cw.manifest.TotalSize
	cw.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("transfer: mkdir dest: %w", err)
	}
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("transfer: create dest: %w", err)
	}
	defer out.Close()

	buf := make([]byte, ChunkSize)
	var written int64
	for i := 0; i < count; i++ {
		chunkPath := filepath.Join(cacheDir, chunkFileName(i))
		f, err := os.Open(chunkPath)
		if err != nil {
			return fmt.Errorf("transfer: open chunk %d: %w", i, err)
		}
		n, err := io.CopyBuffer(out, f, buf)
		f.Close()
		if err != nil {
			return fmt.Errorf("transfer: copy chunk %d: %w", i, err)
		}
		written += n
	}
	if written != total {
		return fmt.Errorf("transfer: assembled size %d != expected %d", written, total)
	}
	return out.Sync()
}

// Cleanup removes the cache directory and meta file. Safe to call multiple times.
func (cw *ChunkWriter) Cleanup() error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	os.RemoveAll(cw.cacheDir)
	return os.Remove(cw.metaPath)
}

func chunkFileName(idx int) string {
	return fmt.Sprintf("chunk_%d", idx)
}
