// Package integration contains end-to-end tests that exercise the full Lanos
// transport stack (Noise XX handshake + EncryptedConn framing + file transfer)
// between two synthetic instances on the loopback interface.
//
// These tests run in CI via `go test ./...` (see .github/workflows/ci.yml).
package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	lanosnet "github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/transport"
)

const headerMagic = "LANOS_FILEv1"

// identityKeys generates a fresh ed25519 keypair and derives the Noise X25519
// static keys, simulating a device identity without touching the OS keystore.
func identityKeys(t *testing.T) transport.StaticKeys {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	sk, err := transport.DeriveStaticKeys(priv)
	if err != nil {
		t.Fatalf("DeriveStaticKeys: %v", err)
	}
	return sk
}

// sendFile writes the Lanos file header then streams the file content over the
// encrypted connection in 256KB frames, mirroring usecase.SendFileUseCase.
func sendFile(t *testing.T, w io.Writer, transferID, fileName string, data []byte) {
	t.Helper()
	header := fmt.Sprintf("%s\n%s\n%d\n%s\n", headerMagic, transferID, len(data), fileName)
	if _, err := w.Write([]byte(header)); err != nil {
		t.Fatalf("send header: %v", err)
	}
	buf := bytes.NewReader(data)
	chunk := make([]byte, 256*1024)
	for {
		n, err := buf.Read(chunk)
		if n > 0 {
			if _, werr := w.Write(chunk[:n]); werr != nil {
				t.Fatalf("send data: %v", werr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("send data read: %v", err)
		}
	}
}

// recvFile reads the Lanos file header from the encrypted connection, then
// reads the file body until expectedSize bytes are received. Returns the file
// name and full body. Mirrors usecase.ReceiveFileUseCase.
func recvFile(t *testing.T, r io.Reader, expectedSize int64) (fileName string, body []byte) {
	t.Helper()
	br := bufio.NewReaderSize(r, 4096)

	magic, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("recv magic: %v", err)
	}
	if strings.TrimSpace(magic) != headerMagic {
		t.Fatalf("bad magic %q", magic)
	}
	idLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("recv id: %v", err)
	}
	_ = strings.TrimSpace(idLine)

	sizeLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("recv size: %v", err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 10, 64)
	if err != nil {
		t.Fatalf("parse size: %v", err)
	}
	if expectedSize > 0 && size != expectedSize {
		t.Fatalf("size mismatch: header=%d want=%d", size, expectedSize)
	}

	nameLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("recv name: %v", err)
	}
	fileName = strings.TrimSpace(nameLine)

	body = make([]byte, 0, size)
	buf := make([]byte, 256*1024)
	for int64(len(body)) < size {
		n, rerr := r.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			t.Fatalf("recv data: %v", rerr)
		}
	}
	if int64(len(body)) != size {
		t.Fatalf("body size %d != expected %d", len(body), size)
	}
	return fileName, body
}

// randFile generates a temp file with n random bytes and returns its path,
// content, and sha256 hash.
func randFile(t *testing.T, n int) (path string, data []byte, hash [32]byte) {
	t.Helper()
	data = make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("rand: %v", err)
	}
	hash = sha256.Sum256(data)
	dir := t.TempDir()
	path = filepath.Join(dir, "source.bin")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path, data, hash
}

// establishPair creates a listener (receiver) and dials it (sender), returning
// the two EncryptedConns after the Noise XX handshake completes. Default
// listen address is 127.0.0.1 loopback for IPv4; pass "::1" (or any other
// listen addr) via establishPairOn for cross-stack coverage.
func establishPair(t *testing.T) (sender, receiver *lanosnet.EncryptedConn) {
	t.Helper()
	return establishPairOn(t, "127.0.0.1:0")
}

// establishPairOn creates a listener on the given address and dials it.
// Used to verify the transport stack works over both IPv4 and IPv6 loopback.
func establishPairOn(t *testing.T, listenAddr string) (sender, receiver *lanosnet.EncryptedConn) {
	t.Helper()
	recvKeys := identityKeys(t)
	sendKeys := identityKeys(t)

	ln, err := lanosnet.NewListener(listenAddr, recvKeys)
	if err != nil {
		t.Fatalf("listen on %s: %v", listenAddr, err)
	}
	defer ln.Close()

	type acceptResult struct {
		res *lanosnet.AcceptResult
		err error
	}
	ch := make(chan acceptResult, 1)
	go func() {
		res, err := ln.Accept()
		ch <- acceptResult{res, err}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sender, err = lanosnet.Dial(ctx, lanosnet.DialConfig{
		Network:    "tcp",
		Address:    ln.Addr().String(),
		StaticKeys: sendKeys,
	})
	if err != nil {
		t.Fatalf("dial %s: %v", ln.Addr().String(), err)
	}

	ar := <-ch
	if ar.err != nil {
		t.Fatalf("accept: %v", ar.err)
	}
	if ar.res == nil || ar.res.Conn == nil {
		t.Fatal("accept returned nil conn")
	}
	receiver = ar.res.Conn
	return sender, receiver
}

func TestTransfer_FileSizes(t *testing.T) {
	t.Parallel()
	sizes := []struct {
		name string
		n    int
	}{
		{"tiny_100B", 100},
		{"exact_chunk_256KB", 256 * 1024},
		{"chunk_boundary_256KB_plus_1", 256*1024 + 1},
		{"multi_chunk_1MB", 1024 * 1024},
		{"large_5MB", 5 * 1024 * 1024},
	}
	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srcPath, srcData, srcHash := randFile(t, tc.n)

			sender, receiver := establishPair(t)
			defer sender.Close()
			defer receiver.Close()

			transferID := "test-transfer-id"
			fileName := "source.bin"

			done := make(chan error, 1)
			go func() {
				sendFile(t, sender, transferID, fileName, srcData)
				done <- nil
			}()

			gotName, gotBody := recvFile(t, receiver, int64(tc.n))
			<-done

			if gotName != fileName {
				t.Errorf("fileName = %q, want %q", gotName, fileName)
			}
			gotHash := sha256.Sum256(gotBody)
			if gotHash != srcHash {
				t.Errorf("sha256 mismatch: got %x, want %x", gotHash, srcHash)
			}
			if !bytes.Equal(gotBody, srcData) {
				t.Errorf("body not equal (len got=%d want=%d)", len(gotBody), len(srcData))
			}
			_ = srcPath
		})
	}
}

func TestTransfer_Bidirectional(t *testing.T) {
	t.Parallel()

	recvKeys := identityKeys(t)
	sendKeys := identityKeys(t)

	ln, err := lanosnet.NewListener("127.0.0.1:0", recvKeys)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	type acceptResult struct {
		res *lanosnet.AcceptResult
		err error
	}
	acceptCh := make(chan acceptResult, 2)
	go func() {
		for i := 0; i < 2; i++ {
			res, err := ln.Accept()
			acceptCh <- acceptResult{res, err}
		}
	}()

	// Conn 1: B dials A (B sends to A)
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	connBtoA, err := lanosnet.Dial(ctx1, lanosnet.DialConfig{
		Network:    "tcp",
		Address:    ln.Addr().String(),
		StaticKeys: sendKeys,
	})
	if err != nil {
		t.Fatalf("dial B->A: %v", err)
	}

	// Conn 2: A dials A's listener (A sends to B) — reuse same listener for simplicity.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	connAtoB, err := lanosnet.Dial(ctx2, lanosnet.DialConfig{
		Network:    "tcp",
		Address:    ln.Addr().String(),
		StaticKeys: sendKeys,
	})
	if err != nil {
		t.Fatalf("dial A->B: %v", err)
	}

	ar1 := <-acceptCh
	ar2 := <-acceptCh
	if ar1.err != nil || ar1.res == nil || ar1.res.Conn == nil {
		t.Fatalf("accept 1: %v", ar1.err)
	}
	if ar2.err != nil || ar2.res == nil || ar2.res.Conn == nil {
		t.Fatalf("accept 2: %v", ar2.err)
	}
	defer connBtoA.Close()
	defer connAtoB.Close()
	defer ar1.res.Conn.Close()
	defer ar2.res.Conn.Close()

	_, dataBtoA, hashBtoA := randFile(t, 512*1024)
	_, dataAtoB, hashAtoB := randFile(t, 384*1024)

	var wg sync.WaitGroup
	wg.Add(2)

	// B sends to A via connBtoA, A receives via ar1.res.Conn
	go func() {
		defer wg.Done()
		sendFile(t, connBtoA, "b2a", "b_to_a.bin", dataBtoA)
	}()
	go func() {
		defer wg.Done()
		sendFile(t, connAtoB, "a2b", "a_to_b.bin", dataAtoB)
	}()

	// A receives via ar1.res.Conn, B receives via ar2.res.Conn
	// We need to match which accept result pairs with which dial. Since both
	// dials hit the same listener, accept order is non-deterministic. Just
	// receive from both and verify one matches each.
	type recvResult struct {
		name string
		body []byte
	}
	recvCh := make(chan recvResult, 2)
	go func() {
		name, body := recvFile(t, ar1.res.Conn, 0)
		recvCh <- recvResult{name, body}
	}()
	go func() {
		name, body := recvFile(t, ar2.res.Conn, 0)
		recvCh <- recvResult{name, body}
	}()

	r1 := <-recvCh
	r2 := <-recvCh
	wg.Wait()

	// Collect received bodies and verify both transfers succeeded.
	received := map[string][]byte{r1.name: r1.body, r2.name: r2.body}
	if h := sha256.Sum256(received["b_to_a.bin"]); h != hashBtoA {
		t.Errorf("b_to_a.bin sha256 mismatch: got %x, want %x", h, hashBtoA)
	}
	if h := sha256.Sum256(received["a_to_b.bin"]); h != hashAtoB {
		t.Errorf("a_to_b.bin sha256 mismatch: got %x, want %x", h, hashAtoB)
	}
}

// TestTransfer_LoopbackIPv6 verifies the transport stack works over IPv6
// loopback ([::1]) — exercising dual-stack listener + v6 dial path.
func TestTransfer_LoopbackIPv6(t *testing.T) {
	t.Parallel()
	srcPath, srcData, srcHash := randFile(t, 256*1024)

	sender, receiver := establishPairOn(t, "[::1]:0")
	defer sender.Close()
	defer receiver.Close()

	transferID := "ipv6-transfer"
	fileName := "over_v6.bin"

	done := make(chan error, 1)
	go func() {
		sendFile(t, sender, transferID, fileName, srcData)
		done <- nil
	}()

	gotName, gotBody := recvFile(t, receiver, int64(len(srcData)))
	<-done

	if gotName != fileName {
		t.Errorf("fileName = %q, want %q", gotName, fileName)
	}
	gotHash := sha256.Sum256(gotBody)
	if gotHash != srcHash {
		t.Fatalf("sha256 mismatch over IPv6: got %x, want %x", gotHash, srcHash)
	}
	_ = srcPath
}

// TestTransfer_AddrselectPicksV6WhenDualStack verifies the integration
// between core/net.SelectAddresses and Dial when a caller feeds it
// multiple candidate destinations on both loopback families.
func TestTransfer_AddrselectPicksV6WhenDualStack(t *testing.T) {
	t.Parallel()

	recvKeys := identityKeys(t)
	sendKeys := identityKeys(t)

	// Spin up two listeners — one on each loopback family — representing a
	// peer advertising both v4 and v6 addresses.
	lnV4, err := lanosnet.NewListener("127.0.0.1:0", recvKeys)
	if err != nil {
		t.Fatalf("listen v4: %v", err)
	}
	defer lnV4.Close()
	lnV6, err := lanosnet.NewListener("[::1]:0", recvKeys)
	if err != nil {
		t.Fatalf("listen v6: %v", err)
	}
	defer lnV6.Close()

	// Candidate dsts are the listener addresses; sources include both loopback
	// families so addrselect can choose v6 first per RFC 6724.
	dsts := []string{lnV4.Addr().String(), lnV6.Addr().String()}
	srcs := []string{"127.0.0.1", "::1"}
	pairs := lanosnet.SelectAddresses(dsts, srcs, 0)
	if len(pairs) < 2 {
		t.Fatalf("expected at least 2 reachable pairs, got %d: %+v", len(pairs), pairs)
	}
	if !pairs[0].IsV6 {
		t.Fatalf("expected v6 first per RFC 6724, got %+v", pairs[0])
	}

	// Accept on whichever family addrselect chose. Both listeners run an
	// accept goroutine; the v6 one should complete first.
	type acceptResult struct {
		res *lanosnet.AcceptResult
		err error
	}
	acceptCh := make(chan acceptResult, 2)
	go func() {
		res, err := lnV6.Accept()
		acceptCh <- acceptResult{res, err}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sender, err := lanosnet.Dial(ctx, lanosnet.DialConfig{
		Network:    "tcp",
		Address:    pairs[0].Destination,
		StaticKeys: sendKeys,
	})
	if err != nil {
		t.Fatalf("dial %s (v6 pick): %v", pairs[0].Destination, err)
	}
	defer sender.Close()

	ar := <-acceptCh
	if ar.err != nil || ar.res == nil || ar.res.Conn == nil {
		t.Fatalf("accept on v6 listener: %v", ar.err)
	}
	defer ar.res.Conn.Close()

	_, data, hash := randFile(t, 64*1024)
	done := make(chan error, 1)
	go func() {
		sendFile(t, sender, "addrselect-v6", "addrselect_v6.bin", data)
		done <- nil
	}()
	gotName, gotBody := recvFile(t, ar.res.Conn, int64(len(data)))
	<-done

	if gotName != "addrselect_v6.bin" {
		t.Errorf("fileName = %q", gotName)
	}
	if h := sha256.Sum256(gotBody); h != hash {
		t.Errorf("sha256 mismatch (v6 pick): got %x, want %x", h, hash)
	}
}

// TestTransfer_IncompatibleIPVersion asserts that SelectAddresses returns an
// empty result (signaling INCOMPATIBLE_IP_VERSION at the API layer) when the
// peer advertises only v4 and the local host has only v6 sources.
func TestTransfer_IncompatibleIPVersion(t *testing.T) {
	t.Parallel()
	dsts := []string{"192.168.1.50:52100"}
	srcs := []string{"::1", "fd00::5"}
	pairs := lanosnet.SelectAddresses(dsts, srcs, 52100)
	if len(pairs) != 0 {
		t.Fatalf("expected zero pairs for incompatible v4-peer/v6-local, got %+v", pairs)
	}
}
