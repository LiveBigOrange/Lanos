package instance

import (
	"os"
	"os/exec"
	"testing"
)

// TestAcquireAndRelease_DoubleAcquireFails verifies that a second Acquire in
// the same process is rejected with ErrAlreadyRunning, and that Release
// allows a subsequent Acquire to succeed.
func TestAcquireAndRelease_DoubleAcquireFails(t *testing.T) {
	// Use a private temp dir so we don't collide with a real gcd running
	// on this machine. We override the lock path by chdir-ing... but
	// Acquire() uses os.TempDir() directly. Instead we set TMPDIR / TEMP.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	// Windows uses TEMP; setting it here is harmless on Unix.
	t.Setenv("TEMP", tmp)

	lock1, err := Acquire()
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	// Second Acquire in the same process must fail. On Unix, flock locks
	// are per-open-file-description, so opening the lock file again and
	// flock-ing it returns EWOULDBLOCK. On Windows, LockFileEx on a fresh
	// handle to the same file fails with ERROR_LOCK_VIOLATION.
	lock2, err := Acquire()
	if err != ErrAlreadyRunning {
		t.Fatalf("second Acquire: err=%v, want ErrAlreadyRunning (lock2=%v)", err, lock2)
	}
	if lock2 != nil {
		t.Fatal("second Acquire returned non-nil Lock")
	}

	if err := lock1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}

	// After Release, Acquire should succeed again.
	lock3, err := Acquire()
	if err != nil {
		t.Fatalf("third Acquire after Release: %v", err)
	}
	if err := lock3.Release(); err != nil {
		t.Fatalf("Release lock3: %v", err)
	}
}

// TestAcquire_SecondProcessFails is the DoD test for PRD §4.4 "双开第二次自动退出":
// it spawns a child process (the test binary itself) that tries to Acquire
// while the parent holds the lock, and verifies the child fails.
//
// The child is invoked with LANOS_INSTANCE_HELPER=1 which triggers
// runHelper() in TestMain.
func TestAcquire_SecondProcessFails(t *testing.T) {
	if os.Getenv("LANOS_INSTANCE_HELPER") == "1" {
		// We are the child: try to acquire and exit with status 0 on
		// success, 1 on ErrAlreadyRunning, 2 on other error.
		runHelperChild()
		return
	}

	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TEMP", tmp)

	// Parent: acquire the lock.
	lock, err := Acquire()
	if err != nil {
		t.Fatalf("parent Acquire: %v", err)
	}
	defer lock.Release()

	// Spawn a child that shares the same TMPDIR/TEMP and tries to Acquire.
	cmd := exec.Command(os.Args[0], "-test.run=TestAcquire_SecondProcessFails")
	cmd.Env = append(os.Environ(), "LANOS_INSTANCE_HELPER=1", "TMPDIR="+tmp, "TEMP="+tmp)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err == nil {
		t.Fatal("child Acquire succeeded; expected failure (process should exit non-zero)")
	}
	// The child exits with status 1 when it sees ErrAlreadyRunning.
	if exit, ok := err.(*exec.ExitError); ok {
		if exit.ExitCode() != 1 {
			t.Errorf("child exit code = %d, want 1 (ErrAlreadyRunning)", exit.ExitCode())
		}
	} else {
		t.Fatalf("child Run error: %v", err)
	}
}

// runHelperChild is executed by the subprocess spawned by
// TestAcquire_SecondProcessFails. It tries to Acquire and exits with:
//
//	0 = acquired (unexpected - means locking is broken)
//	1 = ErrAlreadyRunning (expected - lock held by parent)
//	2 = other error
func runHelperChild() {
	_, err := Acquire()
	if err == ErrAlreadyRunning {
		os.Exit(1)
	}
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}
