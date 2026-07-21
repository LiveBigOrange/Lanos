package bind

import (
	"context"

	"github.com/lanos/lanos/core/usecase"
)

type fakeSender struct {
	called bool
	err    error
	cfg    usecase.SendConfig
}

func (f *fakeSender) Send(_ context.Context, cfg usecase.SendConfig) error {
	f.called = true
	f.cfg = cfg
	return f.err
}

type fakeReceiver struct {
	called bool
	err    error
}

func (f *fakeReceiver) Receive(_ context.Context, _ usecase.ReceiveConfig) error {
	f.called = true
	return f.err
}
