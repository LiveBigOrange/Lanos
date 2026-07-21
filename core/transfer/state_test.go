package transfer

import (
	"errors"
	"testing"
)

func TestStateMachine_HappyPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		path  []Status
		final Status
	}{
		{"pending->connecting->transferring->completed",
			[]Status{StatusConnecting, StatusTransferring, StatusCompleted}, StatusCompleted},
		{"pending->cancelled (user rejected)",
			[]Status{StatusCancelled}, StatusCancelled},
		{"pending->failed (timeout)",
			[]Status{StatusFailed}, StatusFailed},
		{"connecting->failed (dial error)",
			[]Status{StatusConnecting, StatusFailed}, StatusFailed},
		{"transferring->failed (IO error)",
			[]Status{StatusConnecting, StatusTransferring, StatusFailed}, StatusFailed},
		{"transferring->awaiting_resume->connecting->transferring->completed (resume)",
			[]Status{StatusConnecting, StatusTransferring, StatusAwaitingResume, StatusConnecting, StatusTransferring, StatusCompleted}, StatusCompleted},
		{"awaiting_resume->cancelled (user gives up)",
			[]Status{StatusConnecting, StatusTransferring, StatusAwaitingResume, StatusCancelled}, StatusCancelled},
		{"failed->connecting (manual retry)",
			[]Status{StatusFailed, StatusConnecting}, StatusConnecting},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sm := NewStateMachine(StatusPending)
			for _, s := range tc.path {
				if err := sm.Transition(s); err != nil {
					t.Fatalf("Transition(%s): %v", s, err)
				}
			}
			if sm.State() != tc.final {
				t.Fatalf("final state = %s, want %s", sm.State(), tc.final)
			}
		})
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from Status
		to   Status
	}{
		{StatusCompleted, StatusConnecting},
		{StatusCompleted, StatusFailed},
		{StatusCompleted, StatusCancelled},
		{StatusCancelled, StatusConnecting},
		{StatusCancelled, StatusCompleted},
		{StatusPending, StatusTransferring},
		{StatusPending, StatusCompleted},
		{StatusPending, StatusAwaitingResume},
		{StatusConnecting, StatusCompleted},
		{StatusConnecting, StatusPending},
		{StatusTransferring, StatusPending},
		{StatusTransferring, StatusConnecting},
		{StatusAwaitingResume, StatusTransferring},
		{StatusAwaitingResume, StatusCompleted},
		{StatusAwaitingResume, StatusPending},
		{StatusFailed, StatusCompleted},
		{StatusFailed, StatusCancelled},
		{StatusFailed, StatusAwaitingResume},
	}
	for _, tc := range cases {
		t.Run(string(tc.from)+"->"+string(tc.to), func(t *testing.T) {
			t.Parallel()
			sm := NewStateMachine(tc.from)
			err := sm.Transition(tc.to)
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("Transition(%s->%s): err=%v, want ErrInvalidTransition", tc.from, tc.to, err)
			}
			if sm.State() != tc.from {
				t.Fatalf("state changed on invalid transition: got %s, want %s", sm.State(), tc.from)
			}
		})
	}
}

func TestStateMachine_Terminal(t *testing.T) {
	t.Parallel()
	terminal := []Status{StatusCompleted, StatusCancelled}
	nonTerminal := []Status{StatusPending, StatusConnecting, StatusTransferring, StatusAwaitingResume, StatusFailed}
	for _, s := range terminal {
		if !NewStateMachine(s).IsTerminal() {
			t.Errorf("IsTerminal(%s) = false, want true", s)
		}
	}
	for _, s := range nonTerminal {
		if NewStateMachine(s).IsTerminal() {
			t.Errorf("IsTerminal(%s) = true, want false", s)
		}
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		from Status
		want []Status
	}{
		{StatusPending, []Status{StatusCancelled, StatusConnecting, StatusFailed}},
		{StatusConnecting, []Status{StatusAwaitingResume, StatusCancelled, StatusFailed, StatusTransferring}},
		{StatusTransferring, []Status{StatusAwaitingResume, StatusCancelled, StatusCompleted, StatusFailed}},
		{StatusAwaitingResume, []Status{StatusCancelled, StatusConnecting, StatusFailed}},
		{StatusFailed, []Status{StatusConnecting}},
		{StatusCompleted, []Status{}},
		{StatusCancelled, []Status{}},
	}
	for _, tc := range cases {
		t.Run(string(tc.from), func(t *testing.T) {
			t.Parallel()
			got := NewStateMachine(tc.from).ValidTransitions()
			if len(got) != len(tc.want) {
				t.Fatalf("ValidTransitions(%s) = %v, want %v", tc.from, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ValidTransitions(%s)[%d] = %s, want %s", tc.from, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestStateMachine_CanTransition(t *testing.T) {
	t.Parallel()
	sm := NewStateMachine(StatusPending)
	if !sm.CanTransition(StatusConnecting) {
		t.Error("CanTransition(pending->connecting) = false, want true")
	}
	if sm.CanTransition(StatusTransferring) {
		t.Error("CanTransition(pending->transferring) = true, want false")
	}
}

func TestStateMachine_SameStateRejected(t *testing.T) {
	t.Parallel()
	sm := NewStateMachine(StatusTransferring)
	err := sm.Transition(StatusTransferring)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Transition to same state should be rejected by SM (manager handles idempotency), got: %v", err)
	}
}

func TestManager_UpdateStatus_Idempotent(t *testing.T) {
	t.Parallel()
	m := NewManager(3)
	tr, _ := m.Create("peer-1", "Peer", "/x/y.bin", "y.bin", 100)
	_, _ = m.UpdateStatus(tr.ID, StatusConnecting, "")
	// Same status update is a no-op, no error.
	t2, err := m.UpdateStatus(tr.ID, StatusConnecting, "")
	if err != nil {
		t.Fatalf("idempotent UpdateStatus: %v", err)
	}
	if t2.Status != StatusConnecting {
		t.Fatalf("status = %s, want connecting", t2.Status)
	}
}
