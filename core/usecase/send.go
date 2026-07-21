package usecase

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

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
	// TransferID is optional. If non-empty, Execute skips Create and uses
	// the existing transfer (created by the caller). This lets the API
	// handler return the transfer ID immediately while sending runs async.
	TransferID string
}

// PeerNotRespondingMsg is the user-facing error shown when the peer does not
// respond to the transfer within the timeout window (P1-23).
const PeerNotRespondingMsg = "对方未响应"

type SendFileUseCase struct {
	transferMgr *transfer.Manager
	CancelReg   *transfer.CancelRegistry
}

func NewSendFileUseCase(transferMgr *transfer.Manager) *SendFileUseCase {
	return &SendFileUseCase{
		transferMgr: transferMgr,
		CancelReg:   transferMgr.CancelReg,
	}
}

// Send is the contract method of the Sender interface (usecase/interfaces.go).
// It is an alias for Execute and simply forwards the call so callers using
// the interface abstraction reach the same code path.
func (uc *SendFileUseCase) Send(ctx context.Context, cfg SendConfig) error {
	return uc.Execute(ctx, cfg)
}

func (uc *SendFileUseCase) Execute(ctx context.Context, cfg SendConfig) error {
	info, err := os.Stat(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("usecase: stat file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("usecase: directories not yet supported")
	}

	var t *transfer.Transfer
	if cfg.TransferID != "" {
		t, err = uc.transferMgr.Get(cfg.TransferID)
		if err != nil {
			return fmt.Errorf("usecase: get transfer: %w", err)
		}
	} else {
		t, err = uc.transferMgr.Create(cfg.PeerID, cfg.PeerName, cfg.FilePath, info.Name(), info.Size())
		if err != nil {
			return fmt.Errorf("usecase: create transfer: %w", err)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if uc.CancelReg != nil {
		uc.CancelReg.Register(t.ID, cancel, nil)
		defer uc.CancelReg.Complete(t.ID)
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
		select {
		case <-ctx.Done():
			uc.transferMgr.UpdateStatus(t.ID, transfer.StatusCancelled, "")
			return ctx.Err()
		default:
		}
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, PeerNotRespondingMsg)
		return fmt.Errorf("usecase: %s: %w", PeerNotRespondingMsg, err)
	}
	defer enc.Close()

	uc.transferMgr.UpdateStatus(t.ID, transfer.StatusTransferring, "")

	if err := uc.sendHeader(enc, t.ID, info.Name(), info.Size()); err != nil {
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, err.Error())
		return fmt.Errorf("usecase: send header: %w", err)
	}

	sent, err := uc.streamFile(ctx, enc, cfg.FilePath, t.ID)
	if err != nil {
		select {
		case <-ctx.Done():
			uc.transferMgr.UpdateStatus(t.ID, transfer.StatusCancelled, "")
			return ctx.Err()
		default:
		}
		uc.transferMgr.UpdateStatus(t.ID, transfer.StatusFailed, err.Error())
		return fmt.Errorf("usecase: stream file: %w", err)
	}

	uc.transferMgr.UpdateProgress(t.ID, sent)
	uc.transferMgr.UpdateStatus(t.ID, transfer.StatusCompleted, "")
	slog.Info("file sent", "peer", cfg.PeerName, "file", info.Name(), "bytes", sent)
	return nil
}

func (uc *SendFileUseCase) sendHeader(w io.Writer, transferID, fileName string, fileSize int64) error {
	// The receiver reads this header line-by-line (bufio.ReadString('\n')).
	// A fileName containing '\n' or '\r' would break framing: the receiver
	// would treat the trailing part as the start of file body data, or
	// reject the header outright. Reject any such name up-front. The caller
	// (Execute) passes info.Name() from os.Stat, which on every supported
	// platform excludes raw '/' but is permitted to contain control bytes.
	if strings.ContainsAny(fileName, "\n\r") {
		return fmt.Errorf("usecase: send header: file name contains newline: %q", fileName)
	}
	header := fmt.Sprintf("LANOS_FILEv1\n%s\n%d\n%s\n", transferID, fileSize, fileName)
	_, err := w.Write([]byte(header))
	return err
}

func (uc *SendFileUseCase) streamFile(ctx context.Context, w io.Writer, path, transferID string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf := make([]byte, 256*1024)
	var total int64
	var lastProgress int64
	const progressInterval = 256 * 1024
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		n, err := f.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return total, werr
			}
			total += int64(n)
			if total-lastProgress >= progressInterval {
				uc.transferMgr.UpdateProgress(transferID, total)
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
	return total, nil
}
