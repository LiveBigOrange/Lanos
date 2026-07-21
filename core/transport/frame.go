package transport

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// Frame types per docs/PROTOCOL.md §4.2.
const (
	FrameData   uint8 = 0x01
	FrameMeta   uint8 = 0x02
	FrameACK    uint8 = 0x03
	FrameEnd    uint8 = 0x04
	FrameCancel uint8 = 0x05
	FrameError  uint8 = 0x06
)

// FrameHeaderLen is the fixed 16-byte frame header length (§4.2).
const FrameHeaderLen = 16

// MaxPayloadLen is the largest single-frame payload. 4 MiB chunk + slack.
const MaxPayloadLen = 4<<20 + 1024

// Frame protocol errors.
var (
	ErrCRCMismatch     = errors.New("transport: frame crc32 mismatch")
	ErrReservedNonzero = errors.New("transport: reserved bytes must be zero")
	ErrPayloadTooLarge = errors.New("transport: payload too large")
)

// Frame is a single multiplexed frame on a long-lived Noise connection.
// Header layout (docs/PROTOCOL.md §4.2):
//
//	+--------+--------+--------+--------+
//	| stream_id (4B BE)                 |
//	+--------+--------+--------+--------+
//	| frame_type (1B)                   |
//	+--------+--------+
//	| reserved (2B, must be 0)          |
//	+--------+--------+--------+--------+
//	| payload_len (4B BE)               |
//	+--------+--------+--------+--------+
//	| crc32 (4B BE)                     |
//	+--------+--------+--------+--------+
type Frame struct {
	StreamID uint32
	Type     uint8
	Payload  []byte
}

// EncodeFrame writes a complete frame (header + payload) to w.
func EncodeFrame(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayloadLen {
		return fmt.Errorf("%w: %d", ErrPayloadTooLarge, len(f.Payload))
	}
	header := make([]byte, FrameHeaderLen)
	binary.BigEndian.PutUint32(header[0:4], f.StreamID)
	header[4] = f.Type
	// header[5:7] reserved = 0
	binary.BigEndian.PutUint32(header[7:11], uint32(len(f.Payload)))
	binary.BigEndian.PutUint32(header[11:15], crc32.ChecksumIEEE(f.Payload))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("transport: write frame header: %w", err)
	}
	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return fmt.Errorf("transport: write frame payload: %w", err)
		}
	}
	return nil
}

// DecodeFrame reads one complete frame from r and verifies its CRC32.
func DecodeFrame(r io.Reader) (Frame, error) {
	header := make([]byte, FrameHeaderLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, fmt.Errorf("transport: read frame header: %w", err)
	}
	f := Frame{
		StreamID: binary.BigEndian.Uint32(header[0:4]),
		Type:     header[4],
	}
	if header[5] != 0 || header[6] != 0 {
		return Frame{}, ErrReservedNonzero
	}
	payloadLen := binary.BigEndian.Uint32(header[7:11])
	wantCRC := binary.BigEndian.Uint32(header[11:15])
	if payloadLen > MaxPayloadLen {
		return Frame{}, fmt.Errorf("%w: %d", ErrPayloadTooLarge, payloadLen)
	}
	if payloadLen > 0 {
		f.Payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, f.Payload); err != nil {
			return Frame{}, fmt.Errorf("transport: read frame payload: %w", err)
		}
		if crc32.ChecksumIEEE(f.Payload) != wantCRC {
			return Frame{}, ErrCRCMismatch
		}
	}
	return f, nil
}

// MetaPayload is the JSON payload of a META frame (§4.2).
type MetaPayload struct {
	TaskID       string `json:"task_id"`
	FilePath     string `json:"file_path"`
	Size         int64  `json:"size"`
	RelativePath string `json:"relative_path"`
}

// ACKPayload is the JSON payload of an ACK frame.
type ACKPayload struct {
	TaskID   string `json:"task_id"`
	ChunkIdx int64  `json:"chunk_idx"`
	OK       bool   `json:"ok"`
}

// TaskPayload is the JSON payload of END frames.
type TaskPayload struct {
	TaskID string `json:"task_id"`
}

// CancelPayload is the JSON payload of a CANCEL frame.
type CancelPayload struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// ErrorPayload is the JSON payload of an ERROR frame.
type ErrorPayload struct {
	TaskID  string `json:"task_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// EncodePayload marshals a payload struct for use as a Frame payload.
func EncodePayload(v any) ([]byte, error) { return json.Marshal(v) }

// DecodePayload unmarshals a Frame payload into v.
func DecodePayload(p []byte, v any) error { return json.Unmarshal(p, v) }
