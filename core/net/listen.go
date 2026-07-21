package net

import (
	"fmt"
	stdnet "net"
	"sync"

	"github.com/lanos/lanos/core/transport"
)

type Listener struct {
	ln         stdnet.Listener
	staticKeys transport.StaticKeys
	quit       chan struct{}
	wg         sync.WaitGroup
}

func NewListener(addr string, staticKeys transport.StaticKeys) (*Listener, error) {
	raw, err := stdnet.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("net: listen %s: %w", addr, err)
	}
	return &Listener{
		ln:         raw,
		staticKeys: staticKeys,
		quit:       make(chan struct{}),
	}, nil
}

func (l *Listener) Addr() stdnet.Addr {
	return l.ln.Addr()
}

type AcceptResult struct {
	Conn       *EncryptedConn
	PeerStatic []byte
}

func (l *Listener) Accept() (*AcceptResult, error) {
	for {
		raw, err := l.ln.Accept()
		if err != nil {
			select {
			case <-l.quit:
				return nil, nil
			default:
			}
			return nil, fmt.Errorf("net: accept: %w", err)
		}

		hs, err := transport.NewHandshake(transport.RoleResponder, l.staticKeys)
		if err != nil {
			raw.Close()
			continue
		}

		msg1, err := readFrame(raw)
		if err != nil {
			raw.Close()
			continue
		}
		_, _, _, err = hs.ReadMessage(msg1)
		if err != nil {
			raw.Close()
			continue
		}

		msg2, _, _, err := hs.WriteMessage(nil)
		if err != nil {
			raw.Close()
			continue
		}
		if _, err := writeFrame(raw, msg2); err != nil {
			raw.Close()
			continue
		}

		msg3, err := readFrame(raw)
		if err != nil {
			raw.Close()
			continue
		}
		_, sendCS, recvCS, err := hs.ReadMessage(msg3)
		if err != nil {
			raw.Close()
			continue
		}

		return &AcceptResult{
			Conn: &EncryptedConn{
				Conn:   raw,
				sendCS: sendCS,
				recvCS: recvCS,
			},
			PeerStatic: hs.PeerStatic(),
		}, nil
	}
}

func (l *Listener) Close() error {
	close(l.quit)
	l.wg.Wait()
	return l.ln.Close()
}

var PortPicker struct {
	mu   sync.Mutex
	base int
}

func PickPort() int {
	PortPicker.mu.Lock()
	defer PortPicker.mu.Unlock()
	if PortPicker.base == 0 {
		PortPicker.base = 52100
	}
	port := PortPicker.base
	PortPicker.base++
	if PortPicker.base > 52999 {
		PortPicker.base = 52100
	}
	return port
}
