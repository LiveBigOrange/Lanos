package transport

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestEncodeDecodeFrameRoundtrip(t *testing.T) {
	payloads := [][]byte{
		nil,
		{},
		[]byte("hello"),
		make([]byte, 4096),
	}
	for i, p := range payloads {
		f := Frame{StreamID: uint32(i*7 + 1), Type: byte(i%6) + 1, Payload: p}
		var buf bytes.Buffer
		if err := EncodeFrame(&buf, f); err != nil {
			t.Fatalf("case %d: encode: %v", i, err)
		}
		got, err := DecodeFrame(&buf)
		if err != nil {
			t.Fatalf("case %d: decode: %v", i, err)
		}
		if got.StreamID != f.StreamID || got.Type != f.Type {
			t.Fatalf("case %d: header mismatch got=%+v want=%+v", i, got, f)
		}
		if !bytes.Equal(got.Payload, f.Payload) {
			t.Fatalf("case %d: payload mismatch", i)
		}
	}
}

func TestDecodeFrameCRCMismatch(t *testing.T) {
	t.Run("crc", func(t *testing.T) {
		f := Frame{StreamID: 1, Type: FrameData, Payload: []byte("hello")}
		var buf bytes.Buffer
		if err := EncodeFrame(&buf, f); err != nil {
			t.Fatal(err)
		}
		// Flip a payload byte (after the 16-byte header).
		b := buf.Bytes()
		b[16] ^= 0xFF
		_, err := DecodeFrame(bytes.NewReader(b))
		if !errors.Is(err, ErrCRCMismatch) {
			t.Fatalf("got %v, want ErrCRCMismatch", err)
		}
	})
}

func TestDecodeFrameReservedNonzero(t *testing.T) {
	f := Frame{StreamID: 1, Type: FrameMeta, Payload: []byte("x")}
	var buf bytes.Buffer
	if err := EncodeFrame(&buf, f); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	b[5] = 1 // reserved byte
	_, err := DecodeFrame(bytes.NewReader(b))
	if !errors.Is(err, ErrReservedNonzero) {
		t.Fatalf("got %v, want ErrReservedNonzero", err)
	}
}

func TestEncodeFramePayloadTooLarge(t *testing.T) {
	f := Frame{StreamID: 1, Type: FrameData, Payload: make([]byte, MaxPayloadLen+1)}
	var buf bytes.Buffer
	err := EncodeFrame(&buf, f)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("got %v, want ErrPayloadTooLarge", err)
	}
}

func TestEncodeDecodePayloadJSON(t *testing.T) {
	in := MetaPayload{TaskID: "t-1", FilePath: "/a/b.txt", Size: 1234, RelativePath: "b.txt"}
	b, err := EncodePayload(in)
	if err != nil {
		t.Fatal(err)
	}
	var out MetaPayload
	if err := DecodePayload(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("roundtrip mismatch: got %+v want %+v", out, in)
	}
}

// newMuxPair wires two Muxes over a connected net.Pipe pair.
func newMuxPair(t *testing.T) (*Mux, *Mux, func()) {
	t.Helper()
	ca, cb := net.Pipe()
	muxA := NewMux(ca, RoleInitiator)
	muxB := NewMux(cb, RoleResponder)
	go muxA.Serve()
	go muxB.Serve()
	cleanup := func() {
		muxA.Close()
		muxB.Close()
	}
	return muxA, muxB, cleanup
}

// TestConcurrentDualStreams is the P1-14 DoD: a single connection carries two
// concurrent streams transferring two "files" (random bytes) at once.
func TestConcurrentDualStreams(t *testing.T) {
	muxA, muxB, cleanup := newMuxPair(t)
	defer cleanup()

	const numStreams = 2
	const chunkCount = 8
	const chunkSize = 32 * 1024

	// Pre-generate the two "files".
	files := make([][]byte, numStreams)
	for i := range files {
		files[i] = make([]byte, chunkSize*chunkCount)
		if _, err := rand.Read(files[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Receiver side: accept streams, read META + all DATA + END, verify.
	type recvResult struct {
		streamID uint32
		data     []byte
		err      error
	}
	recvCh := make(chan recvResult, numStreams)
	var recvWG sync.WaitGroup
	recvWG.Add(numStreams)
	go func() {
		for i := 0; i < numStreams; i++ {
			st, err := muxB.Accept()
			if err != nil {
				recvCh <- recvResult{err: err}
				recvWG.Done()
				continue
			}
			go func(st *Stream) {
				defer recvWG.Done()
				meta, err := st.RecvFrame()
				if err != nil {
					recvCh <- recvResult{err: err}
					return
				}
				if meta.Type != FrameMeta {
					recvCh <- recvResult{err: errors.New("first frame not META")}
					return
				}
				var buf []byte
				for {
					f, err := st.RecvFrame()
					if err != nil {
						recvCh <- recvResult{err: err}
						return
					}
					if f.Type == FrameEnd {
						break
					}
					if f.Type != FrameData {
						recvCh <- recvResult{err: errors.New("unexpected frame type")}
						return
					}
					buf = append(buf, f.Payload...)
				}
				recvCh <- recvResult{streamID: st.ID(), data: buf}
			}(st)
		}
	}()

	// Sender side: open both streams concurrently, send META + DATA + END.
	var sendWG sync.WaitGroup
	sendWG.Add(numStreams)
	for i := 0; i < numStreams; i++ {
		i := i
		go func() {
			defer sendWG.Done()
			st, err := muxA.Open()
			if err != nil {
				t.Errorf("open stream %d: %v", i, err)
				return
			}
			meta, _ := EncodePayload(MetaPayload{TaskID: "t", FilePath: "f", Size: int64(len(files[i]))})
			if err := st.SendMeta(meta); err != nil {
				t.Errorf("send meta %d: %v", i, err)
				return
			}
			for c := 0; c < chunkCount; c++ {
				start := c * chunkSize
				if err := st.SendData(files[i][start : start+chunkSize]); err != nil {
					t.Errorf("send data %d.%d: %v", i, c, err)
					return
				}
			}
			end, _ := EncodePayload(TaskPayload{TaskID: "t"})
			if err := st.SendEnd(end); err != nil {
				t.Errorf("send end %d: %v", i, err)
			}
		}()
	}
	sendWG.Wait()

	// Collect receiver results.
	results := make([]recvResult, 0, numStreams)
	for i := 0; i < numStreams; i++ {
		select {
		case r := <-recvCh:
			results = append(results, r)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for receiver")
		}
	}

	// Verify both streams delivered intact and match one of the sent files.
	matched := make([]bool, numStreams)
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("receiver error: %v", r.err)
		}
		found := false
		for i, f := range files {
			if matched[i] {
				continue
			}
			if bytes.Equal(r.data, f) {
				matched[i] = true
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("received data matched no sent file (len=%d, streamID=%d)", len(r.data), r.streamID)
		}
	}
	for i, m := range matched {
		if !m {
			t.Errorf("file %d was not received", i)
		}
	}

	// Verify stream ID parity: initiator opened even IDs.
	for _, r := range results {
		if r.streamID%2 != 0 {
			t.Errorf("receiver stream ID %d should be even (initiator-allocated)", r.streamID)
		}
	}
}

// TestStreamIDParity ensures initiator allocates even IDs and responder odd.
func TestStreamIDParity(t *testing.T) {
	muxA, muxB, cleanup := newMuxPair(t)
	defer cleanup()

	// A (initiator) opens a stream -> its ID must be even.
	stA, err := muxA.Open()
	if err != nil {
		t.Fatal(err)
	}
	if stA.ID()%2 != 0 {
		t.Fatalf("initiator stream ID %d not even", stA.ID())
	}

	// Send META so B accepts; then B opens a stream -> its ID must be odd.
	meta, _ := EncodePayload(MetaPayload{TaskID: "x"})
	if err := stA.SendMeta(meta); err != nil {
		t.Fatal(err)
	}
	stB, err := muxB.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if stB.ID() != stA.ID() {
		t.Fatalf("accepted stream ID %d != opened %d", stB.ID(), stA.ID())
	}

	stB2, err := muxB.Open()
	if err != nil {
		t.Fatal(err)
	}
	if stB2.ID()%2 != 1 {
		t.Fatalf("responder stream ID %d not odd", stB2.ID())
	}

	stA2, err := muxA.Open()
	if err != nil {
		t.Fatal(err)
	}
	if stA2.ID()%2 != 0 {
		t.Fatalf("second initiator stream ID %d not even", stA2.ID())
	}
	if stA2.ID() == stA.ID() {
		t.Fatalf("second initiator stream ID reused %d", stA2.ID())
	}
}

// TestStreamEndClosesStream verifies END frame closes the receiver stream.
func TestStreamEndClosesStream(t *testing.T) {
	muxA, muxB, cleanup := newMuxPair(t)
	defer cleanup()

	stA, _ := muxA.Open()
	meta, _ := EncodePayload(MetaPayload{TaskID: "x"})
	stA.SendMeta(meta)

	stB, _ := muxB.Accept()
	// drain META
	_, _ = stB.RecvFrame()

	end, _ := EncodePayload(TaskPayload{TaskID: "x"})
	if err := stA.SendEnd(end); err != nil {
		t.Fatal(err)
	}

	// B should receive END then EOF on next RecvFrame.
	f, err := stB.RecvFrame()
	if err != nil {
		t.Fatalf("expected END frame, got err: %v", err)
	}
	if f.Type != FrameEnd {
		t.Fatalf("expected END, got type %d", f.Type)
	}
	_, err = stB.RecvFrame()
	if !errors.Is(err, ErrStreamClosed) && err != io.EOF {
		t.Fatalf("expected EOF/ErrStreamClosed after END, got %v", err)
	}
}

// TestMuxCloseClosesStreams verifies mux shutdown propagates to all streams.
func TestMuxCloseClosesStreams(t *testing.T) {
	muxA, muxB, cleanup := newMuxPair(t)
	defer cleanup()

	stA, _ := muxA.Open()
	meta, _ := EncodePayload(MetaPayload{TaskID: "x"})
	stA.SendMeta(meta)
	stB, _ := muxB.Accept()

	muxA.Close()

	select {
	case <-stA.Done():
	case <-time.After(time.Second):
		t.Fatal("stA not closed after mux close")
	}
	// B's mux read loop will hit EOF and shut down too.
	select {
	case <-stB.Done():
	case <-time.After(time.Second):
		t.Fatal("stB not closed after peer mux close")
	}
}

// TestLargePayloadRoundtrip verifies a near-max-size payload survives intact.
func TestLargePayloadRoundtrip(t *testing.T) {
	muxA, muxB, cleanup := newMuxPair(t)
	defer cleanup()

	payload := make([]byte, 1024*1024) // 1 MiB
	rand.Read(payload)

	stA, _ := muxA.Open()
	meta, _ := EncodePayload(MetaPayload{TaskID: "x"})
	stA.SendMeta(meta)
	stA.SendData(payload)
	end, _ := EncodePayload(TaskPayload{TaskID: "x"})
	stA.SendEnd(end)

	stB, _ := muxB.Accept()
	_, _ = stB.RecvFrame() // META
	data, err := stB.RecvFrame()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data.Payload, payload) {
		t.Fatalf("large payload corrupted: got len %d want %d", len(data.Payload), len(payload))
	}
	_, _ = stB.RecvFrame() // END
}

// TestUnknownStreamDropped verifies non-META frames to unknown streams are
// silently dropped (not errors) - simulates teardown race.
func TestUnknownStreamDropped(t *testing.T) {
	ca, cb := net.Pipe()
	muxA := NewMux(ca, RoleInitiator)
	muxB := NewMux(cb, RoleResponder)
	go muxA.Serve()
	go muxB.Serve()
	defer muxA.Close()
	defer muxB.Close()

	// Send a DATA frame to a stream B never opened (no preceding META).
	// Encode directly to bypass Mux.Open's registration.
	if err := EncodeFrame(ca, Frame{StreamID: 999, Type: FrameData, Payload: []byte("orphan")}); err != nil {
		t.Fatal(err)
	}
	// B should simply drop it and keep serving. Open a legitimate stream right
	// after to prove the mux is still healthy.
	time.Sleep(50 * time.Millisecond)
	stA, err := muxA.Open()
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := EncodePayload(MetaPayload{TaskID: "x"})
	if err := stA.SendMeta(meta); err != nil {
		t.Fatalf("mux unhealthy after orphan frame: %v", err)
	}
	if _, err := muxB.Accept(); err != nil {
		t.Fatalf("mux B Accept failed after orphan frame: %v", err)
	}
}

// TestPayloadLenFieldBigEndian verifies the payload_len field is big-endian
// (§4.2) by hand-decoding a known frame.
func TestPayloadLenFieldBigEndian(t *testing.T) {
	f := Frame{StreamID: 0x01020304, Type: FrameData, Payload: make([]byte, 300)}
	var buf bytes.Buffer
	EncodeFrame(&buf, f)
	b := buf.Bytes()
	if got := binary.BigEndian.Uint32(b[0:4]); got != 0x01020304 {
		t.Fatalf("stream_id not BE: %x", got)
	}
	if got := binary.BigEndian.Uint32(b[7:11]); got != 300 {
		t.Fatalf("payload_len not BE: %d", got)
	}
}
