package main

import (
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// countingScan returns the key with the number of the call, e.g. "a1", and counts the calls.
type countingScan struct {
	calls atomic.Int32
}

func (c *countingScan) scan(key string) (string, error) {
	return key + strconv.Itoa(int(c.calls.Add(1))), nil
}

func keepAll(string) bool { return true }

func TestCachedReusesResults(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	clock := start
	counter := &countingScan{}
	scan := cached(counter.scan, keepAll, 5*time.Minute, func() time.Time { return clock })

	for _, step := range []struct {
		at        time.Duration // since start
		key, want string
	}{
		{0, "a", "a1"},
		{0, "a", "a1"},
		{time.Minute, "b", "b2"},
		{5*time.Minute - time.Nanosecond, "a", "a1"},
		// Five minutes after a scan, the key is scanned again.
		{5 * time.Minute, "a", "a3"},
		{5 * time.Minute, "b", "b2"},
		{6*time.Minute - time.Nanosecond, "b", "b2"},
		{6 * time.Minute, "b", "b4"},
		{10*time.Minute - time.Nanosecond, "a", "a3"},
		{10 * time.Minute, "a", "a5"},
	} {
		clock = start.Add(step.at)
		if got, err := scan(step.key); got != step.want || err != nil {
			t.Errorf("after %v, scan(%q) = %q, %v; want %q", step.at, step.key, got, err, step.want)
		}
	}
}

func TestCachedDoesNotKeepFailures(t *testing.T) {
	calls := 0
	scan := cached(func(key string) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("unavailable")
		}
		return "ok", nil
	}, keepAll, time.Minute, time.Now)

	if _, err := scan("a"); err == nil || err.Error() != "unavailable" {
		t.Errorf("first scan error = %v; want unavailable", err)
	}
	for range 2 {
		if got, err := scan("a"); got != "ok" || err != nil {
			t.Errorf("scan() = %q, %v; want ok", got, err)
		}
	}
	if calls != 2 {
		t.Errorf("scanned %d times; want 2", calls)
	}
}

func TestCachedKeepsOnlyAcceptedResults(t *testing.T) {
	counter := &countingScan{}
	scan := cached(counter.scan, func(value string) bool { return value != "a1" }, time.Minute, time.Now)

	// The result of the first scan is not accepted, so the second call scans again; its result is kept.
	for _, want := range []string{"a1", "a2", "a2"} {
		if got, err := scan("a"); got != want || err != nil {
			t.Errorf("scan() = %q, %v; want %q", got, err, want)
		}
	}
}

func TestCachedSharesRunningScan(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	calls := atomic.Int32{}
	scan := cached(func(key string) (string, error) {
		calls.Add(1)
		close(started)
		<-release
		return "done", nil
	}, keepAll, time.Minute, time.Now)

	results := make(chan string, 2)
	go func() { got, _ := scan("a"); results <- got }()
	<-started
	go func() { got, _ := scan("a"); results <- got }()

	select {
	case got := <-results:
		t.Fatalf("a scan returned %q before the running scan finished", got)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range 2 {
		if got := <-results; got != "done" {
			t.Errorf("scan() = %q; want done", got)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("scanned %d times; want 1", n)
	}
}

func TestCachedRecoversFromPanickedScan(t *testing.T) {
	calls := 0
	scan := cached(func(key string) (string, error) {
		calls++
		if calls == 1 {
			panic("bug")
		}
		return "ok", nil
	}, keepAll, time.Minute, time.Now)

	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic of the scan was not propagated")
			}
		}()
		scan("a")
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if got, err := scan("a"); got != "ok" || err != nil {
			t.Errorf("scan() after a panic = %q, %v; want ok", got, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scan() after a panic is still waiting for the panicked scan")
	}
}
