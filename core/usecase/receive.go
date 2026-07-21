package usecase

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/receive"
	"github.com/lanos/lanos/core/transport"
)

type ReceiveConfig struct {
	PeerAddr   string
	PeerID     string
	PeerName   string
	SaveDir    string
	StaticKeys transport.StaticKeys
}

type ReceiveFileUseCase struct {
	receiveMgr *receive.Manager
}

func NewReceiveFileUseCase(receiveMgr *receive.Manager) *ReceiveFileUseCase {
	return &ReceiveFileUseCase{receiveMgr: receiveMgr}
}

func (uc *ReceiveFileUseCase) Execute(ctx context.Context, cfg ReceiveConfig) error {
	enc, err := net.Dial(ctx, net.DialConfig{
		Network:    "tcp",
		Address:    cfg.PeerAddr,
		StaticKeys: cfg.StaticKeys,
	})
	if err != nil {
		return fmt.Errorf("usecase: dial: %w", err)
	}
	defer enc.Close()

	transferID, fileName, fileSize, err := uc.readHeader(enc)
	if err != nil {
		return fmt.Errorf("usecase: read header: %w", err)
	}

	inc, err := uc.receiveMgr.Register(cfg.PeerID, cfg.PeerName, fileName, fileSize, 0)
	if err != nil {
		return fmt.Errorf("usecase: register: %w", err)
	}

	savePath := filepath.Join(cfg.SaveDir, fileName)
	inc, err = uc.receiveMgr.Accept(inc.ID, savePath)
	if err != nil {
		uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusFailed, err.Error())
		return fmt.Errorf("usecase: accept: %w", err)
	}

	uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusReceiving, "")

	received, err := uc.saveFile(enc, savePath, fileSize, inc.ID)
	if err != nil {
		uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusFailed, err.Error())
		return fmt.Errorf("usecase: save file: %w", err)
	}

	uc.receiveMgr.UpdateProgress(inc.ID, received)
	uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusCompleted, "")
	slog.Info("file received", "peer", cfg.PeerName, "file", fileName, "bytes", received, "savePath", savePath)
	_ = transferID
	return nil
}

func (uc *ReceiveFileUseCase) readHeader(r io.Reader) (transferID, fileName string, fileSize int64, err error) {
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

	name, err := br.ReadString('\n')
	if err != nil {
		return "", "", 0, fmt.Errorf("read file name: %w", err)
	}
	fileName = strings.TrimSpace(name)

	return transferID, fileName, fileSize, nil
}

func (uc *ReceiveFileUseCase) saveFile(r io.Reader, savePath string, expectedSize int64, incomingID string) (int64, error) {
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
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			uc.receiveMgr.UpdateProgress(incomingID, total)
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
