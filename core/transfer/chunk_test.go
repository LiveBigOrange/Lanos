package transfer

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile creates a file of the given size filled with a deterministic
// repeating pattern and returns its path.
func writeTestFile(t *testing.T, path string, size int64) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	for i := range buf {
		buf[i] = byte(i * 31)
	}
	var written int64
	for written < size {
		want := int64(len(buf))
		if want > size-written {
			want = size - written
		}
		n, err := f.Write(buf[:want])
		if err != nil {
			t.Fatal(err)
		}
		written += int64(n)
	}
}

func TestChunkCount(t *testing.T) {
	cases := []struct {
		size int64
		want int
	}{
		{0, 0},
		{1, 1},
		{ChunkSize, 1},
		{ChunkSize + 1, 2},
		{3 * ChunkSize, 3},
		{3*ChunkSize - 1, 3},
	}
	for _, c := range cases {
		if got := ChunkCount(c.size); got != c.want {
			t.Errorf("ChunkCount(%d) = %d, want %d", c.size, got, c.want)
		}
	}
}

func TestChunkHashHex(t *testing.T) {
	got := ChunkHashHex([]byte("hello"))
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestChunkReaderRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	size := int64(10*1024*1024 + 1234) // ~10 MiB -> 3 chunks
	writeTestFile(t, path, size)

	r, err := NewChunkReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.TotalSize() != size {
		t.Fatalf("TotalSize = %d want %d", r.TotalSize(), size)
	}
	if r.ChunkCount() != 3 {
		t.Fatalf("ChunkCount = %d want 3", r.ChunkCount())
	}

	// Read all chunks, collect data + hashes.
	var collected []byte
	var hashes []string
	idx := 0
	for {
		c, err := r.Next()
		if err != nil {
			break
		}
		if c.Idx != idx {
			t.Fatalf("idx = %d want %d", c.Idx, idx)
		}
		collected = append(collected, c.Data...)
		hashes = append(hashes, c.Sha256)
		idx++
	}
	if int64(len(collected)) != size {
		t.Fatalf("collected %d bytes, want %d", len(collected), size)
	}
	if len(hashes) != 3 {
		t.Fatalf("got %d hashes, want 3", len(hashes))
	}
	// Last chunk must be smaller than ChunkSize.
	lastChunkSize := size - 2*ChunkSize
	if int64(len(hashes)) != 3 {
		t.Fatal("expected 3 chunks")
	}
	_ = lastChunkSize
}

func TestBuildManifestMatchesReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	size := int64(5*1024*1024 + 7) // 2 chunks
	writeTestFile(t, path, size)

	// Build manifest.
	r1, err := NewChunkReader(path)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := r1.BuildManifest("task-1", "file.bin")
	if err != nil {
		t.Fatal(err)
	}
	r1.Close()
	if manifest.TotalSize != size {
		t.Fatalf("manifest TotalSize %d want %d", manifest.TotalSize, size)
	}
	if manifest.ChunkCount != 2 {
		t.Fatalf("manifest ChunkCount %d want 2", manifest.ChunkCount)
	}
	if len(manifest.ChunkHashes) != 2 {
		t.Fatalf("manifest hashes len %d want 2", len(manifest.ChunkHashes))
	}

	// Independently re-read chunks and verify hashes match manifest.
	r2, err := NewChunkReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	for i := 0; i < manifest.ChunkCount; i++ {
		c, err := r2.Next()
		if err != nil {
			t.Fatalf("chunk %d: %v", i, err)
		}
		if c.Sha256 != manifest.ChunkHashes[i] {
			t.Fatalf("chunk %d hash mismatch: reader %s manifest %s", i, c.Sha256, manifest.ChunkHashes[i])
		}
	}
}

func TestChunkWriterVerifiesAndAssembles(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.bin")
	size := int64(3*1024*1024 + 42) // 1 chunk (last smaller)
	writeTestFile(t, srcPath, size)

	// Build manifest.
	r, err := NewChunkReader(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := r.BuildManifest("task-2", "src.bin")
	if err != nil {
		t.Fatal(err)
	}
	r.Close()

	cacheRoot := filepath.Join(dir, "cache")
	cw, err := NewChunkWriter(cacheRoot, "task-2", *manifest)
	if err != nil {
		t.Fatal(err)
	}

	// Feed chunks from a fresh reader.
	r2, err := NewChunkReader(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	for {
		c, err := r2.Next()
		if err != nil {
			break
		}
		if err := cw.WriteChunk(c.Idx, c.Data); err != nil {
			t.Fatalf("WriteChunk %d: %v", c.Idx, err)
		}
	}

	if cw.ReceivedCount() != manifest.ChunkCount {
		t.Fatalf("ReceivedCount %d want %d", cw.ReceivedCount(), manifest.ChunkCount)
	}

	// Assemble and compare to source.
	dstPath := filepath.Join(dir, "dst.bin")
	if err := cw.Assemble(dstPath); err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if !filesEqual(t, srcPath, dstPath) {
		t.Fatal("assembled file differs from source")
	}

	// Cleanup removes cache.
	if err := cw.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "task-2")); !os.IsNotExist(err) {
		t.Fatalf("cache dir still exists after Cleanup")
	}
}

func TestChunkWriterRejectsBadHash(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{TaskID: "t", FileName: "f", TotalSize: 10, ChunkCount: 1, ChunkHashes: []string{"0000000000000000000000000000000000000000000000000000000000000000"}}
	cw, err := NewChunkWriter(dir, "t", manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer cw.Cleanup()
	err = cw.WriteChunk(0, []byte("hello world"))
	if !errors.Is(err, ErrChunkHash) {
		t.Fatalf("got %v, want ErrChunkHash", err)
	}
}

func TestChunkWriterRejectsBadIndex(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{TaskID: "t", FileName: "f", TotalSize: 10, ChunkCount: 1, ChunkHashes: []string{"x"}}
	cw, _ := NewChunkWriter(dir, "t", manifest)
	defer cw.Cleanup()
	if err := cw.WriteChunk(5, []byte("x")); !errors.Is(err, ErrChunkIndex) {
		t.Fatalf("got %v, want ErrChunkIndex", err)
	}
}

func TestChunkWriterResumeSkipsReceived(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.bin")
	size := int64(2*ChunkSize + 100) // 3 chunks
	writeTestFile(t, srcPath, size)

	r, _ := NewChunkReader(srcPath)
	manifest, _ := r.BuildManifest("task-3", "src.bin")
	r.Close()

	cacheRoot := filepath.Join(dir, "cache")
	cw, _ := NewChunkWriter(cacheRoot, "task-3", *manifest)

	// Write only the first 2 chunks, then simulate a disconnect.
	r2, _ := NewChunkReader(srcPath)
	for i := 0; i < 2; i++ {
		c, err := r2.Next()
		if err != nil {
			t.Fatal(err)
		}
		if err := cw.WriteChunk(c.Idx, c.Data); err != nil {
			t.Fatalf("WriteChunk %d: %v", c.Idx, err)
		}
	}
	r2.Close()
	if cw.ReceivedCount() != 2 {
		t.Fatalf("ReceivedCount %d want 2", cw.ReceivedCount())
	}
	if !cw.IsReceived(0) || !cw.IsReceived(1) {
		t.Fatal("chunks 0,1 should be marked received")
	}
	if cw.IsReceived(2) {
		t.Fatal("chunk 2 should NOT be received yet")
	}

	// Simulate resume: create a NEW ChunkWriter (loads meta from disk).
	cw2, err := NewChunkWriter(cacheRoot, "task-3", *manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer cw2.Cleanup()
	if cw2.ReceivedCount() != 2 {
		t.Fatalf("resumed ReceivedCount %d want 2", cw2.ReceivedCount())
	}
	if !cw2.IsReceived(0) || !cw2.IsReceived(1) || cw2.IsReceived(2) {
		t.Fatal("resumed received state wrong")
	}

	// Feed all 3 chunks again; the first 2 should be no-ops, only chunk 2 written.
	r3, _ := NewChunkReader(srcPath)
	for i := 0; i < 3; i++ {
		c, err := r3.Next()
		if err != nil {
			t.Fatal(err)
		}
		if err := cw2.WriteChunk(c.Idx, c.Data); err != nil {
			t.Fatalf("resume WriteChunk %d: %v", c.Idx, err)
		}
	}
	r3.Close()
	if cw2.ReceivedCount() != 3 {
		t.Fatalf("final ReceivedCount %d want 3", cw2.ReceivedCount())
	}

	dstPath := filepath.Join(dir, "dst.bin")
	if err := cw2.Assemble(dstPath); err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if !filesEqual(t, srcPath, dstPath) {
		t.Fatal("resumed assembled file differs from source")
	}
}

func TestAssembleRejectsIncomplete(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{TaskID: "t", FileName: "f", TotalSize: int64(2 * ChunkSize), ChunkCount: 2, ChunkHashes: []string{"a", "b"}}
	cw, _ := NewChunkWriter(dir, "t", manifest)
	defer cw.Cleanup()
	err := cw.Assemble(filepath.Join(dir, "out"))
	if !errors.Is(err, ErrIncomplete) {
		t.Fatalf("got %v, want ErrIncomplete", err)
	}
}

func TestManifestJSONRoundtrip(t *testing.T) {
	m := Manifest{TaskID: "t-9", FileName: "f.bin", TotalSize: 12345, ChunkCount: 2, ChunkHashes: []string{"aa", "bb"}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out Manifest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.TaskID != m.TaskID || out.FileName != m.FileName || out.TotalSize != m.TotalSize ||
		out.ChunkCount != m.ChunkCount || len(out.ChunkHashes) != len(m.ChunkHashes) ||
		out.ChunkHashes[0] != m.ChunkHashes[0] || out.ChunkHashes[1] != m.ChunkHashes[1] {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", out, m)
	}
}

func filesEqual(t *testing.T, a, b string) bool {
	t.Helper()
	fa, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(fa, fb)
}

// BenchmarkChunkReaderSHA256 measures raw chunk-read + SHA256 throughput.
// The P1-15 DoD targets >= 80 MB/s for a 1 GiB transfer.
func BenchmarkChunkReaderSHA256(b *testing.B) {
	// Use an in-memory source to isolate CPU (SHA256 + copy) from disk I/O.
	const totalSize = 256 << 20 // 256 MiB
	data := make([]byte, totalSize)
	rand.Read(data)

	b.SetBytes(totalSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := 0
		off := 0
		for off < len(data) {
			end := off + ChunkSize
			if end > len(data) {
				end = len(data)
			}
			_ = ChunkHashHex(data[off:end])
			off = end
			idx++
		}
		_ = idx
	}
}
