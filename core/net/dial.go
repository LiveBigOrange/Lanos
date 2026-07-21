package net

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	stdnet "net"
	"sync"
	"time"

	"github.com/flynn/noise"
	"github.com/lanos/lanos/core/transport"
)

const (
	DefaultDialTimeout = 10 * time.Second
	frameHeaderLen     = 4
)

type DialConfig struct {
	Network    string
	Address    string
	StaticKeys transport.StaticKeys
	Timeout    time.Duration
}

func (c *DialConfig) defaults() {
	if c.Network == "" {
		c.Network = "tcp"
	}
	if c.Timeout <= 0 {
		c.Timeout = DefaultDialTimeout
	}
}

func Dial(ctx context.Context, cfg DialConfig) (*EncryptedConn, error) {
	cfg.defaults()
	d := stdnet.Dialer{Timeout: cfg.Timeout}
	raw, err := d.DialContext(ctx, cfg.Network, cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("net: dial %s: %w", cfg.Address, err)
	}

	hs, err := transport.NewHandshake(transport.RoleInitiator, cfg.StaticKeys)
	if err != nil {
		raw.Close()
		return nil, err
	}

	msg1, _, _, err := hs.WriteMessage(nil)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: write msg1: %w", err)
	}
	if _, err := writeFrame(raw, msg1); err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: send msg1: %w", err)
	}

	msg2, err := readFrame(raw)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: read msg2: %w", err)
	}
	_, _, _, err = hs.ReadMessage(msg2)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: process msg2: %w", err)
	}

	msg3, sendCS, recvCS, err := hs.WriteMessage(nil)
	if err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: write msg3: %w", err)
	}
	if _, err := writeFrame(raw, msg3); err != nil {
		raw.Close()
		return nil, fmt.Errorf("net: send msg3: %w", err)
	}

	return &EncryptedConn{
		Conn:   raw,
		sendCS: sendCS,
		recvCS: recvCS,
	}, nil
}

type EncryptedConn struct {
	stdnet.Conn
	sendCS  *noise.CipherState
	recvCS  *noise.CipherState
	buf     []byte
	writeMu sync.Mutex
}

func (ec *EncryptedConn) Read(b []byte) (int, error) {
	if len(ec.buf) > 0 {
		n := copy(b, ec.buf)
		ec.buf = ec.buf[n:]
		return n, nil
	}
	frame, err := readFrame(ec.Conn)
	if err != nil {
		return 0, err
	}
	plain, err := ec.recvCS.Decrypt(nil, nil, frame)
	if err != nil {
		return 0, fmt.Errorf("net: decrypt: %w", err)
	}
	n := copy(b, plain)
	if n < len(plain) {
		ec.buf = plain[n:]
	}
	return n, nil
}

func (ec *EncryptedConn) Write(b []byte) (int, error) {
	ec.writeMu.Lock()
	defer ec.writeMu.Unlock()
	ct, err := ec.sendCS.Encrypt(nil, nil, b)
	if err != nil {
		return 0, fmt.Errorf("net: encrypt: %w", err)
	}
	_, err = writeFrame(ec.Conn, ct)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func writeFrame(w io.Writer, data []byte) (int, error) {
	header := make([]byte, frameHeaderLen)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	n, err := w.Write(header)
	if err != nil {
		return n, err
	}
	nn, err := w.Write(data)
	return n + nn, err
}

func readFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, frameHeaderLen)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("net: read frame header: %w", err)
	}
	size := binary.BigEndian.Uint32(header)
	if size > 16<<20 {
		return nil, fmt.Errorf("net: frame too large: %d", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("net: read frame body: %w", err)
	}
	return data, nil
}
