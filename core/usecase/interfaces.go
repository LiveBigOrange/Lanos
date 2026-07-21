package usecase

import "context"

// Sender is the outbound transfer usecase contract (P4-W12 layer abstraction).
//
// Implementations:
//   - *SendFileUseCase — production implementation wired to *transfer.Manager.
//
// The contract exists so the API layer, mobile bind entry, and tests can
// depend on the use case boundary rather than the concrete struct. The
// implementation also keeps an Execute method aliasing Send for callers that
// pre-date this abstraction; both are equivalent.
type Sender interface {
	Send(ctx context.Context, cfg SendConfig) error
}

// Receiver is the inbound transfer usecase contract (P4-W12 layer abstraction).
//
// Implementations:
//   - *ReceiveFileUseCase — production implementation wired to *receive.Manager.
//
// See Sender docs for rationale. Receive is an alias for the existing Execute
// method on *ReceiveFileUseCase.
type Receiver interface {
	Receive(ctx context.Context, cfg ReceiveConfig) error
}

// Compile-time contract assertions.
var (
	_ Sender   = (*SendFileUseCase)(nil)
	_ Receiver = (*ReceiveFileUseCase)(nil)
)
