package filelock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusiveAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lease")
	first, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := Try(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Wait(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait %v", err)
	}
	first.Close()
	last, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	last.Close()
}

func TestKernelReleasesLockAfterCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.lock")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockChild$")
	cmd.Env = append(os.Environ(), "HUDDLE_TEST_LOCK="+path)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	if line, err := bufio.NewReader(pipe).ReadString('\n'); err != nil || line != "held\n" {
		t.Fatalf("child readiness %q %v", line, err)
	}
	if _, err := Try(path); !errors.Is(err, ErrBusy) {
		t.Fatalf("child did not hold lock: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait() // a deliberately killed fixture exits unsuccessfully
	l, err := Try(path)
	if err != nil {
		t.Fatalf("crash left stale lock: %v", err)
	}
	l.Close()
}
func TestLockChild(t *testing.T) {
	path := os.Getenv("HUDDLE_TEST_LOCK")
	if path == "" {
		t.Skip("subprocess helper")
	}
	l, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fmt.Println("held")
	for {
		time.Sleep(time.Second)
	}
}
