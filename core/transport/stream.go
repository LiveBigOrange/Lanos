// Package transport: stream multiplexer.
//
// Mux multiplexes multiple logical Streams over a single Noise EncryptedConn
// using the 16-byte frame protocol from docs/PROTOCOL.md §4.2. Stream ID
// allocation follows §4.4: the connection initiator allocates even stream IDs
// (0, 2, 4, ...) and the responder allocates odd IDs (1, 3, 5, ...). This lets
// A->B and B->A transfers share one long connection without collision.
//
// Streams are frame-oriented, not byte-oriented: the transfer layer sends META,
// then DATA frames (one per chunk), then END; the receiver reads frames via
// RecvFrame. This matches the protocol exactly and avoids double-framing.
package transport

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// Mux/stream errors.
var (
	ErrMuxClosed     = errors.New("transport: mux closed")
	ErrStreamClosed  = errors.New("transport: stream closed")
	ErrStreamExists  = errors.New("transport: stream already exists")
	ErrUnknownStream = errors.New("transport: unknown stream")
)

// frameRecvBuf is the per-stream inbound frame buffer size. Large enough that a
// fast sender rarely blocks, small enough to provide backpressure.
const frameRecvBuf = 128

// Mux multiplexes Streams over one reliable ordered byte connection.
type Mux struct {
	conn     io.ReadWriteCloser
	role     HandshakeRole
	writeMu  sync.Mutex // serializes frame writes onto the wire

	mu       sync.Mutex
	streams  map[uint32]*Stream
	nextID   uint32
	closed   bool
	closeErr error
	doneCh   chan struct{}

	acceptCh chan *Stream
}

// NewMux wraps conn. role determines stream-ID parity (initiator=even,
// responder=odd) per §4.4. Call Serve once to start the read loop.
func NewMux(conn io.ReadWriteCloser, role HandshakeRole) *Mux {
	startID := uint32(0)
	if role == RoleResponder {
		startID = 1 // odd
	}
	return &Mux{
		conn:     conn,
		role:     role,
		streams:  make(map[uint32]*Stream),
		nextID:   startID,
		doneCh:   make(chan struct{}),
		acceptCh: make(chan *Stream, 16),
	}
}

// Serve runs the read loop, dispatching inbound frames until the connection
// closes. Serve blocks; run it in a goroutine. On return all streams are closed
// and Accept returns the close error.
func (m *Mux) Serve() {
	defer m.shutdown(io.EOF)
	for {
		f, err := DecodeFrame(m.conn)
		if err != nil {
			m.shutdown(err)
			return
		}
		m.dispatch(f)
	}
}

// dispatch routes a frame to its stream. A META frame on an unknown stream ID
// opens a new inbound stream, delivered via Accept.
func (m *Mux) dispatch(f Frame) {
	m.mu.Lock()
	st, ok := m.streams[f.StreamID]
	if !ok && f.Type == FrameMeta {
		st = newStream(f.StreamID, m)
		m.streams[f.StreamID] = st
		m.mu.Unlock()
		select {
		case m.acceptCh <- st:
		case <-m.doneCh:
			st.close(ErrMuxClosed)
			return
		}
		st.deliver(f)
		return
	}
	m.mu.Unlock()
	if !ok {
		return // unknown stream + non-META: drop (peer may have torn down already)
	}
	st.deliver(f)
}

// Open allocates a new outbound Stream with the next stream ID for this role.
func (m *Mux) Open() (*Stream, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrMuxClosed
	}
	id := m.nextID
	m.nextID += 2 // preserve parity
	st := newStream(id, m)
	m.streams[id] = st
	m.mu.Unlock()
	return st, nil
}

// Accept blocks until a peer opens a stream (via a META frame) or the Mux closes.
func (m *Mux) Accept() (*Stream, error) {
	select {
	case st := <-m.acceptCh:
		return st, nil
	case <-m.doneCh:
		return nil, m.closeErr
	}
}

// writeFrame serializes all outbound frames onto the wire.
func (m *Mux) writeFrame(f Frame) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrMuxClosed
	}
	m.mu.Unlock()
	if err := EncodeFrame(m.conn, f); err != nil {
		m.shutdown(err)
		return err
	}
	return nil
}

// removeStream drops a stream from the registry.
func (m *Mux) removeStream(id uint32) {
	m.mu.Lock()
	delete(m.streams, id)
	m.mu.Unlock()
}

// Close shuts down the mux and the underlying connection.
func (m *Mux) Close() error {
	m.shutdown(ErrMuxClosed)
	return nil
}

// shutdown closes the connection and all streams. Idempotent.
func (m *Mux) shutdown(err error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.closeErr = err
	streams := make([]*Stream, 0, len(m.streams))
	for _, st := range m.streams {
		streams = append(streams, st)
	}
	m.mu.Unlock()

	close(m.doneCh)
	m.conn.Close()
	for _, st := range streams {
		st.close(err)
	}
}

// Done returns a channel closed when the Mux shuts down.
func (m *Mux) Done() <-chan struct{} { return m.doneCh }

// --- Stream ---

// Stream is a single multiplexed logical stream. It is frame-oriented: use
// SendFrame/RecvFrame (or the typed helpers SendData/SendMeta/SendEnd/...) to
// exchange whole frames. Receiving an END, CANCEL, or ERROR frame closes the
// stream after the frame is returned from RecvFrame.
type Stream struct {
	id        uint32
	mux       *Mux
	rcvCh     chan Frame
	closed    atomic.Bool
	closeErr  error
	closeOnce sync.Once
	doneCh    chan struct{}
}

func newStream(id uint32, m *Mux) *Stream {
	return &Stream{
		id:    id,
		mux:   m,
		rcvCh: make(chan Frame, frameRecvBuf),
		doneCh: make(chan struct{}),
	}
}

// ID returns the stream's 32-bit stream ID.
func (s *Stream) ID() uint32 { return s.id }

// deliver is called by the Mux read loop for each inbound frame on this stream.
func (s *Stream) deliver(f Frame) {
	if s.closed.Load() {
		return
	}
	select {
	case s.rcvCh <- f:
	case <-s.doneCh:
	}
}

// RecvFrame returns the next inbound frame. It blocks until a frame arrives or
// the stream closes. END/CANCEL/ERROR frames close the stream after being
// returned, so callers will see the control frame once then EOF on next call.
func (s *Stream) RecvFrame() (Frame, error) {
	select {
	case f := <-s.rcvCh:
		if f.Type == FrameEnd || f.Type == FrameCancel || f.Type == FrameError {
			s.close(nil)
		}
		return f, nil
	case <-s.doneCh:
		return Frame{}, s.closeErr
	}
}

// SendFrame writes a frame on this stream (StreamID is forced to this stream's
// ID).
func (s *Stream) SendFrame(ftype uint8, payload []byte) error {
	if s.closed.Load() {
		return ErrStreamClosed
	}
	return s.mux.writeFrame(Frame{StreamID: s.id, Type: ftype, Payload: payload})
}

// SendData writes a DATA frame.
func (s *Stream) SendData(payload []byte) error { return s.SendFrame(FrameData, payload) }

// SendMeta writes a META frame. The first frame on a new outbound stream MUST
// be META so the peer's Mux opens the inbound stream.
func (s *Stream) SendMeta(payload []byte) error { return s.SendFrame(FrameMeta, payload) }

// SendACK writes an ACK frame.
func (s *Stream) SendACK(payload []byte) error { return s.SendFrame(FrameACK, payload) }

// SendEnd writes an END frame and closes the local side.
func (s *Stream) SendEnd(payload []byte) error {
	if err := s.SendFrame(FrameEnd, payload); err != nil {
		return err
	}
	s.close(nil)
	return nil
}

// SendCancel writes a CANCEL frame and closes the local side.
func (s *Stream) SendCancel(payload []byte) error {
	if err := s.SendFrame(FrameCancel, payload); err != nil {
		return err
	}
	s.close(nil)
	return nil
}

// SendError writes an ERROR frame and closes the local side.
func (s *Stream) SendError(payload []byte) error {
	if err := s.SendFrame(FrameError, payload); err != nil {
		return err
	}
	s.close(nil)
	return nil
}

// Close closes the local side of the stream without sending a teardown frame.
// Use SendEnd/SendCancel/SendError to notify the peer.
func (s *Stream) Close() error {
	s.close(nil)
	return nil
}

// close marks the stream closed and drains waiters. Idempotent.
func (s *Stream) close(err error) {
	s.closeOnce.Do(func() {
		if err == nil {
			err = ErrStreamClosed
		}
		s.closed.Store(true)
		s.closeErr = err
		close(s.doneCh)
		s.mux.removeStream(s.id)
	})
}

// Done returns a channel closed when the stream closes.
func (s *Stream) Done() <-chan struct{} { return s.doneCh }
