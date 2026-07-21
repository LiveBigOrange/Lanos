package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

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

func (uc *ReceiveFileUseCase) Receive(ctx context.Context, cfg ReceiveConfig) error {
	return uc.Execute(ctx, cfg)
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

	transferID, fileName, fileSize, err := receive.ReadHeader(enc)
	if err != nil {
		return fmt.Errorf("usecase: read header: %w", err)
	}

	inc, err := uc.receiveMgr.Register(cfg.PeerID, cfg.PeerName, fileName, fileSize, 0)
	if err != nil {
		return fmt.Errorf("usecase: register: %w", err)
	}

	slog.Info("incoming transfer header parsed", "transferID", transferID, "fileName", fileName, "fileSize", fileSize)

	savePath := filepath.Join(cfg.SaveDir, filepath.Base(fileName))
	inc, err = uc.receiveMgr.Accept(inc.ID, savePath)
	if err != nil {
		uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusFailed, err.Error())
		return fmt.Errorf("usecase: accept: %w", err)
	}

	uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusReceiving, "")

	received, err := receive.SaveFile(enc, savePath, fileSize, inc.ID, uc.receiveMgr)
	if err != nil {
		uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusFailed, err.Error())
		return fmt.Errorf("usecase: save file: %w", err)
	}

	uc.receiveMgr.UpdateProgress(inc.ID, received)
	uc.receiveMgr.UpdateStatus(inc.ID, receive.StatusCompleted, "")
	slog.Info("file received", "peer", cfg.PeerName, "file", fileName, "bytes", received, "savePath", savePath, "transferID", transferID)
	return nil
}
