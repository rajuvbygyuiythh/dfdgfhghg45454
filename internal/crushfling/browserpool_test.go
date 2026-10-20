package crushfling

// The pool is what makes the dashboard's Stop button close every browser.

import (
	"sync"
	"testing"
)

type fakeBrowser struct {
	mu     sync.Mutex
	killed int
}

func (f *fakeBrowser) Kill() {
	f.mu.Lock()
	f.killed++
	f.mu.Unlock()
}

func (f *fakeBrowser) kills() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.killed
}

func TestBrowserPoolTracksAndClosesAll(t *testing.T) {
	before := OpenBrowserCount()
	a, b := &fakeBrowser{}, &fakeBrowser{}
	releaseA := trackBrowser(a)
	releaseB := trackBrowser(b)
	if got := OpenBrowserCount(); got != before+2 {
		t.Fatalf("open = %d, want %d", got, before+2)
	}
	if n := CloseAllBrowsers(); n < 2 {
		t.Fatalf("CloseAllBrowsers closed %d, want >= 2", n)
	}
	if a.kills() != 1 || b.kills() != 1 {
		t.Fatalf("kills = %d/%d, want 1/1", a.kills(), b.kills())
	}
	if got := OpenBrowserCount(); got != before {
		t.Fatalf("pool not emptied: %d", got)
	}
	// The deferred release runs after a normal finish: it must be harmless
	// even though CloseAllBrowsers already removed the entries.
	releaseA()
	releaseB()
	if a.kills() != 2 {
		t.Fatalf("release should also kill an already-closed browser once: %d", a.kills())
	}
	// Nothing tracked: closing is a no-op, not an error.
	if n := CloseAllBrowsers(); n != 0 {
		t.Fatalf("empty pool closed %d", n)
	}
}

func TestBrowserPoolIgnoresNil(t *testing.T) {
	release := trackBrowser(nil)
	release()
	if got := OpenBrowserCount(); got != 0 {
		t.Fatalf("nil handle registered: %d", got)
	}
}
