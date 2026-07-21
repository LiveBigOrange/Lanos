package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetShare(t *testing.T) {
	db := openTestDB(t)
	exp := time.Now().Add(1 * time.Hour)
	maxDl := 5
	r := &ShareRecord{
		ID: "s1", Kind: "link", Target: "http://192.168.1.1:52103/dl/abc",
		FilePath: "/tmp/test.txt", Size: 1024, Status: "active",
		CreatedAt: time.Now(), ExpiresAt: &exp, Downloads: 0, MaxDownloads: &maxDl,
	}
	if err := db.CreateShare(r); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	got, err := db.GetShare("s1")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if got.ID != "s1" || got.Kind != "link" || got.Size != 1024 {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.MaxDownloads == nil || *got.MaxDownloads != 5 {
		t.Fatalf("maxDownloads = %v", got.MaxDownloads)
	}
}

func TestUpdateShareStatus(t *testing.T) {
	db := openTestDB(t)
	db.CreateShare(&ShareRecord{ID: "s1", Kind: "link", FilePath: "/f", Size: 1, Status: "active", CreatedAt: time.Now()})
	if err := db.UpdateShareStatus("s1", "stopped"); err != nil {
		t.Fatalf("UpdateShareStatus: %v", err)
	}
	got, _ := db.GetShare("s1")
	if got.Status != "stopped" {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestIncrementShareDownloads(t *testing.T) {
	db := openTestDB(t)
	db.CreateShare(&ShareRecord{ID: "s1", Kind: "link", FilePath: "/f", Size: 1, Status: "active", CreatedAt: time.Now()})
	db.IncrementShareDownloads("s1")
	db.IncrementShareDownloads("s1")
	got, _ := db.GetShare("s1")
	if got.Downloads != 2 {
		t.Fatalf("downloads = %d", got.Downloads)
	}
}

func TestDeleteShare(t *testing.T) {
	db := openTestDB(t)
	db.CreateShare(&ShareRecord{ID: "s1", Kind: "link", FilePath: "/f", Size: 1, Status: "active", CreatedAt: time.Now()})
	if err := db.DeleteShare("s1"); err != nil {
		t.Fatalf("DeleteShare: %v", err)
	}
	_, err := db.GetShare("s1")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestListShares(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		db.CreateShare(&ShareRecord{
			ID: string(rune('a' + i)), Kind: "link", FilePath: "/tmp/file" + string(rune('a'+i)) + ".txt",
			Size: int64(100 * (i + 1)), Status: "active", CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	// List all
	list, err := db.ListShares(DefaultListOptions())
	if err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("len = %d", len(list))
	}
	// Search
	list, _ = db.ListShares(ListOptions{Search: "fileb", SortBy: SortByTime, Order: SortDesc, Limit: 10})
	if len(list) != 1 {
		t.Fatalf("search len = %d", len(list))
	}
	// Sort by size ascending
	list, _ = db.ListShares(ListOptions{SortBy: SortBySize, Order: SortAsc, Limit: 10})
	if len(list) != 5 {
		t.Fatalf("sort len = %d", len(list))
	}
	if list[0].Size != 100 {
		t.Fatalf("first size = %d", list[0].Size)
	}
}

func TestCreateAndGetTransfer(t *testing.T) {
	db := openTestDB(t)
	fin := time.Now()
	r := &TransferRecord{
		ID: "t1", Direction: "send", PeerDeviceID: "dev-abc", PeerName: "MacBook",
		FilePath: "/tmp/doc.pdf", Size: 2048, Status: "completed",
		StartedAt: time.Now().Add(-5 * time.Minute), FinishedAt: &fin,
	}
	if err := db.CreateTransfer(r); err != nil {
		t.Fatalf("CreateTransfer: %v", err)
	}
	got, err := db.GetTransfer("t1")
	if err != nil {
		t.Fatalf("GetTransfer: %v", err)
	}
	if got.PeerName != "MacBook" || got.Status != "completed" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestUpdateTransferStatus(t *testing.T) {
	db := openTestDB(t)
	db.CreateTransfer(&TransferRecord{ID: "t1", Direction: "send", PeerDeviceID: "d", FilePath: "/f", Size: 1, Status: "active", StartedAt: time.Now()})
	fin := time.Now()
	if err := db.UpdateTransferStatus("t1", "completed", &fin, ""); err != nil {
		t.Fatalf("UpdateTransferStatus: %v", err)
	}
	got, _ := db.GetTransfer("t1")
	if got.Status != "completed" || got.FinishedAt == nil {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestListTransfers(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		db.CreateTransfer(&TransferRecord{
			ID: string(rune('a' + i)), Direction: "recv", PeerDeviceID: "d1", PeerName: "Device",
			FilePath: "/recv/file" + string(rune('a'+i)) + ".zip", Size: int64(500 * (i + 1)),
			Status: "completed", StartedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	list, err := db.ListTransfers(DefaultListOptions())
	if err != nil {
		t.Fatalf("ListTransfers: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("len = %d", len(list))
	}
	// Search
	list, _ = db.ListTransfers(ListOptions{Search: "filec", SortBy: SortByTime, Order: SortDesc, Limit: 10})
	if len(list) != 1 {
		t.Fatalf("search len = %d", len(list))
	}
}

func TestDeleteTransfersBatch(t *testing.T) {
	db := openTestDB(t)
	for _, id := range []string{"t1", "t2", "t3"} {
		db.CreateTransfer(&TransferRecord{ID: id, Direction: "send", PeerDeviceID: "d", FilePath: "/f", Size: 1, Status: "completed", StartedAt: time.Now()})
	}
	if err := db.DeleteTransfers([]string{"t1", "t3"}); err != nil {
		t.Fatalf("DeleteTransfers: %v", err)
	}
	n, _ := db.CountTransfers()
	if n != 1 {
		t.Fatalf("remaining = %d", n)
	}
}

func TestEnforceRetention(t *testing.T) {
	db := openTestDB(t)
	now := time.Now()
	for i := 0; i < 10; i++ {
		db.CreateShare(&ShareRecord{
			ID: string(rune('a' + i)), Kind: "link", FilePath: "/f", Size: 1, Status: "completed",
			CreatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	if err := db.EnforceRetention(3); err != nil {
		t.Fatalf("EnforceRetention: %v", err)
	}
	n, _ := db.CountShares()
	if n != 3 {
		t.Fatalf("shares after retention = %d, want 3", n)
	}
}

func TestExportSharesCSV(t *testing.T) {
	db := openTestDB(t)
	db.CreateShare(&ShareRecord{ID: "s1", Kind: "link", Target: "http://x", FilePath: "/tmp/a.txt", Size: 100, Status: "active", CreatedAt: time.Now()})
	var buf bytes.Buffer
	if err := db.ExportSharesCSV(&buf); err != nil {
		t.Fatalf("ExportSharesCSV: %v", err)
	}
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("s1")) {
		t.Fatalf("csv missing id: %s", out)
	}
	if !bytes.Contains(buf.Bytes(), []byte("/tmp/a.txt")) {
		t.Fatalf("csv missing path: %s", out)
	}
}

func TestExportTransfersCSV(t *testing.T) {
	db := openTestDB(t)
	db.CreateTransfer(&TransferRecord{ID: "t1", Direction: "send", PeerDeviceID: "d", PeerName: "Mac", FilePath: "/tmp/b.zip", Size: 200, Status: "completed", StartedAt: time.Now()})
	var buf bytes.Buffer
	if err := db.ExportTransfersCSV(&buf); err != nil {
		t.Fatalf("ExportTransfersCSV: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("t1")) {
		t.Fatalf("csv missing id")
	}
}

func TestOpenCreatesFile(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()
	if _, err := os.Stat(filepath.Join(dir, "transfer_log.db")); os.IsNotExist(err) {
		t.Fatal("db file not created")
	}
}

func TestNullTimeRoundTrip(t *testing.T) {
	db := openTestDB(t)
	// With nil expiry
	db.CreateShare(&ShareRecord{ID: "s1", Kind: "link", FilePath: "/f", Size: 1, Status: "active", CreatedAt: time.Now()})
	got, _ := db.GetShare("s1")
	if got.ExpiresAt != nil {
		t.Fatal("ExpiresAt should be nil")
	}
	// With expiry
	exp := time.Now().Add(2 * time.Hour)
	db.CreateShare(&ShareRecord{ID: "s2", Kind: "link", FilePath: "/f", Size: 1, Status: "active", CreatedAt: time.Now(), ExpiresAt: &exp})
	got2, _ := db.GetShare("s2")
	if got2.ExpiresAt == nil {
		t.Fatal("ExpiresAt should not be nil")
	}
}
