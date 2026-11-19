package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakePool struct {
	mu      sync.Mutex
	failing bool
	resets  int
	pings   int
}

func (f *fakePool) Ping(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings++
	if f.failing {
		return errors.New("fora do ar")
	}
	return nil
}
func (f *fakePool) Reset()        { f.mu.Lock(); f.resets++; f.mu.Unlock() }
func (f *fakePool) set(fail bool) { f.mu.Lock(); f.failing = fail; f.mu.Unlock() }
func (f *fakePool) get() (resets, pings int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resets, f.pings
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condição não ocorreu a tempo")
}

func TestWatchResetsAfterConsecutiveFailures(t *testing.T) {
	p := &fakePool{failing: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, p, WatchOptions{Every: 3 * time.Millisecond, ResetAfter: 3, GiveUpAfter: time.Hour})
	waitFor(t, func() bool { r, _ := p.get(); return r >= 2 }) // 6 falhas = 2 resets
}

func TestWatchHealthyNeverResets(t *testing.T) {
	p := &fakePool{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, p, WatchOptions{Every: 2 * time.Millisecond})
	waitFor(t, func() bool { _, n := p.get(); return n >= 10 })
	if r, _ := p.get(); r != 0 {
		t.Fatalf("resets = %d, want 0", r)
	}
}

func TestWatchRecoveryResetsTheCounter(t *testing.T) {
	p := &fakePool{failing: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, p, WatchOptions{Every: 3 * time.Millisecond, ResetAfter: 3, GiveUpAfter: time.Hour})
	waitFor(t, func() bool { r, _ := p.get(); return r >= 1 })
	p.set(false)
	_, n0 := p.get()
	waitFor(t, func() bool { _, n := p.get(); return n >= n0+10 })
	r1, _ := p.get()
	time.Sleep(20 * time.Millisecond)
	if r2, _ := p.get(); r2 != r1 {
		t.Fatalf("resets seguiram subindo com o banco saudável: %d -> %d", r1, r2)
	}
}

func TestWatchGivesUpOnceAndOnlyWhenStillDown(t *testing.T) {
	p := &fakePool{failing: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	gave := 0
	go Watch(ctx, p, WatchOptions{Every: 3 * time.Millisecond, ResetAfter: 2, GiveUpAfter: 30 * time.Millisecond,
		OnGiveUp: func() { mu.Lock(); gave++; mu.Unlock() }})
	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return gave >= 1 })
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gave != 1 {
		t.Fatalf("OnGiveUp chamado %d vezes, want 1", gave)
	}
}

func TestWatchDoesNotGiveUpIfItRecovers(t *testing.T) {
	p := &fakePool{failing: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	gave := 0
	go Watch(ctx, p, WatchOptions{Every: 3 * time.Millisecond, GiveUpAfter: 200 * time.Millisecond,
		OnGiveUp: func() { mu.Lock(); gave++; mu.Unlock() }})
	time.Sleep(30 * time.Millisecond)
	p.set(false)
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if gave != 0 {
		t.Fatalf("desistiu mesmo com o banco de volta (%d)", gave)
	}
}

func TestWatchStopsWithContext(t *testing.T) {
	p := &fakePool{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { Watch(ctx, p, WatchOptions{Every: time.Millisecond}); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Watch não parou com o contexto")
	}
}
