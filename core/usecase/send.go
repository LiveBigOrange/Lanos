package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/lanos/lanos/core/net"
	"github.com/lanos/lanos/core/transfer"
	"github.com/lanos/lanos/core/transport"
)

type SendConfig struct {
	PeerID     string
	PeerAddr   string
	PeerName   string
	FilePath   string
	StaticKeys transport.StaticKeys
}

type SendFileUseCase struct {
	transferMgr *transfer.Manager
}

func NewSendFileUseCase(transferMgr *transfer.Manager) *SendFileUseCase {
	return &SendFileUseCase{transferMgr: transferMgr}
}

func (uc *SendFileUseCase) Execute(ctx context.Context, cfg SendConfig) error {
	info, err := os.Stat(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("usecase: stat file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("usecase: directories not yet supported")
	}

	t, err := uc.transferMgr.Create(cfg.PeerID, cfg.PeerName, cfg.FilePath, info.Name(), info.Size())
	if err != nil {
		return fmt.Errorf("usecase: create transfer: %w", err)
	}

	t, err = uc.transferMgr.UpdateStatus(t.ID, transfer.StatusConnecting, "")
	if err != nil {
		return err
	}

	enc, err := net.Dial(ctx, net.DialConfig{
		Network:    "tcp",
		Address:    cfg.PeerAddr,
		StaticKeys: cfg.StaticKeys,
	})
	if err != nil {
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, err.Error())
		return fmt.Errorf("usecase: dial: %w", err)
	}
	defer enc.Close()

	uc.transferMgr.UpdateStatus(t.ID, transfer.StatusTransferring, "")

	if err := uc.sendHeader(enc, t.ID, info.Name(), info.Size()); err != nil {
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, err.Error())
		return fmt.Errorf("usecase: send header: %w", err)
	}

	sent, err := uc.streamFile(enc, cfg.FilePath, t.ID)
	if err != nil {
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, err.Error())
		return fmt.Errorf("usecase: stream file: %w", err)
	}

	uc.transferMgr.UpdateProgress(t.ID, sent)
	uc.transferMgr.UpdateStatus(t.ID, transfer.StatusCompleted, "")
	slog.Info("file sent", "peer", cfg.PeerName, "file", info.Name(), "bytes", sent)
	return nil
}

func (uc *SendFileUseCase) sendHeader(w io.Writer, transferID, fileName string, fileSize int64) error {
	header := fmt.Sprintf("LANOS_FILEv1\n%s\n%d\n%s\n", transferID, fileSize, fileName)
	_, err := w.Write([]byte(header))
	return err
}

func (uc *SendFileUseCase) streamFile(w io.Writer, path, transferID string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 256*1024)
	var total int64
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			uc.transferMgr.UpdateProgress(transferID, total)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
