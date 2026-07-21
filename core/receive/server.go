package receive

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	corenet "github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/transport"
)

type InboundServer struct {
	ln         *corenet.Listener
	receiveMgr *Manager
	staticKeys transport.StaticKeys
	quit       chan struct{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewInboundServer(addr string, receiveMgr *Manager, staticKeys transport.StaticKeys) (*InboundServer, error) {
	ln, err := corenet.NewListener(addr, staticKeys)
	if err != nil {
		return nil, fmt.Errorf("inbound: new listener: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &InboundServer{
		ln:         ln,
		receiveMgr: receiveMgr,
		staticKeys: staticKeys,
		quit:       make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

func (s *InboundServer) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			result, err := s.ln.Accept()
			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					slog.Error("inbound: accept error", "err", err)
					continue
				}
			}
			if result == nil {
				return
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleConn(s.ctx, result.Conn, result.PeerStatic)
			}()
		}
	}()
}

func (s *InboundServer) Addr() string {
	return s.ln.Addr().String()
}

func (s *InboundServer) Port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *InboundServer) Close() {
	close(s.quit)
	s.cancel()
	s.ln.Close()
	s.wg.Wait()
}

func (s *InboundServer) handleConn(ctx context.Context, conn *corenet.EncryptedConn, peerStatic []byte) {
	defer conn.Close()

	peerID := fmt.Sprintf("%x", peerStatic[:8])
	_, fileName, fileSize, err := ReadHeader(conn)
	if err != nil {
		slog.Error("inbound: read header", "err", err)
		return
	}

	inc, err := s.receiveMgr.Register(peerID, "unknown", fileName, fileSize, 0)
	if err != nil {
		slog.Error("inbound: register", "err", err)
		return
	}

	s.receiveMgr.UpdateStatus(inc.ID, StatusPrompting, "")

	status, err := s.waitForAccept(ctx, inc.ID)
	if err != nil {
		slog.Error("inbound: wait accept", "err", err)
		return
	}

	switch status {
	case StatusRejected, StatusCancelled, StatusExpired:
		return
	case StatusAccepting:
	default:
		return
	}

	s.receiveMgr.UpdateStatus(inc.ID, StatusReceiving, "")

	inc2, err := s.receiveMgr.Get(inc.ID)
	if err != nil {
		s.receiveMgr.UpdateStatus(inc.ID, StatusFailed, "transfer lost during accept")
		return
	}
	savePath := inc2.SavePath
	if savePath == "" {
		savePath = filepath.Join(os.TempDir(), filepath.Base(fileName))
	}

	received, err := SaveFile(conn, savePath, fileSize, inc.ID, s.receiveMgr)
	if err != nil {
		s.receiveMgr.UpdateStatus(inc.ID, StatusFailed, err.Error())
		return
	}

	s.receiveMgr.UpdateProgress(inc.ID, received)
	s.receiveMgr.UpdateStatus(inc.ID, StatusCompleted, "")
	slog.Info("inbound: file received", "id", inc.ID[:8], "file", fileName, "bytes", received, "savePath", savePath)
}

func (s *InboundServer) waitForAccept(ctx context.Context, id string) (Status, error) {
	notify := s.receiveMgr.NotifyCh()
	for {
		inc, err := s.receiveMgr.Get(id)
		if err != nil {
			return "", err
		}
		switch inc.Status {
		case StatusAccepting:
			return StatusAccepting, nil
		case StatusRejected:
			return StatusRejected, nil
		case StatusCancelled:
			return StatusCancelled, nil
		case StatusExpired:
			return StatusExpired, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-s.quit:
			return StatusCancelled, nil
		case <-notify:
		}
	}
}

func ReadHeader(r io.Reader) (transferID, fileName string, fileSize int64, err error) {
	br := bufio.NewReaderSize(r, 4096)

	magic, err := br.ReadString('\n')
	if err != nil {
		return "", "", 0, fmt.Errorf("read magic: %w", err)
	}
	magic = strings.TrimSpace(magic)
	if magic != "LANOS_FILEv1" {
		return "", "", 0, fmt.Errorf("unknown protocol: %s", magic)
	}

	id, err := br.ReadString('\n')
	if err != nil {
		return "", "", 0, fmt.Errorf("read transfer id: %w", err)
	}
	transferID = strings.TrimSpace(id)

	sizeLine, err := br.ReadString('\n')
	if err != nil {
		return "", "", 0, fmt.Errorf("read file size: %w", err)
	}
	fileSize, err = strconv.ParseInt(strings.TrimSpace(sizeLine), 10, 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("parse file size: %w", err)
	}
	if fileSize < 0 {
		return "", "", 0, fmt.Errorf("invalid file size: %d", fileSize)
	}

	name, err := br.ReadString('\n')
	if err != nil {
		return "", "", 0, fmt.Errorf("read file name: %w", err)
	}
	fileName = strings.TrimSpace(name)
	if len(fileName) == 0 || len(fileName) > 4096 {
		return "", "", 0, fmt.Errorf("invalid file name length: %d", len(fileName))
	}

	return transferID, fileName, fileSize, nil
}

func SaveFile(r io.Reader, savePath string, expectedSize int64, incomingID string, mgr *Manager) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(savePath), 0o755); err != nil {
		return 0, fmt.Errorf("create save dir: %w", err)
	}

	f, err := os.Create(savePath)
	if err != nil {
		return 0, fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 256*1024)
	var total int64
	var lastProgress int64
	const progressInterval = 256 * 1024
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			if total-lastProgress >= progressInterval {
				mgr.UpdateProgress(incomingID, total)
				lastProgress = total
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
	}

	if expectedSize > 0 && total != expectedSize {
		return total, fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, total)
	}
	return total, nil
}
