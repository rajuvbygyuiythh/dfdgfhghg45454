package crushfling

// browserpool.go — bookkeeping for every Chromium this process launches.
//
// Why it exists: "Stop" must leave nothing running. Cancelling the context
// makes the worker stop asking for more work, but a browser that is already
// open (waiting on a Cloudflare page, for example) has no reason to exit on
// its own. Each launch is therefore registered here, and StopBulk calls
// CloseAllBrowsers() so the windows really close.
//
// The interface deliberately does not import go-rod: tests and the sandbox
// build can supply a fake handle, and this file compiles without the browser
// dependency.

import "sync"

// browserHandle is the only thing the pool needs from a browser: the ability
// to kill its process. *launcher.Launcher satisfies it.
type browserHandle interface {
	Kill()
}

type browserPool struct {
	mu   sync.Mutex
	next int
	open map[int]browserHandle
}

var browsers = &browserPool{open: map[int]browserHandle{}}

// trackBrowser registers a launched browser and returns the function that
// unregisters it. Call the returned function with defer, next to the existing
// Cleanup call:
//
//	defer trackBrowser(l)()
func trackBrowser(h browserHandle) func() {
	if h == nil {
		return func() {}
	}
	browsers.mu.Lock()
	browsers.next++
	id := browsers.next
	browsers.open[id] = h
	browsers.mu.Unlock()
	return func() {
		browsers.mu.Lock()
		delete(browsers.open, id)
		browsers.mu.Unlock()
		// Kill is a no-op for a process that already exited, so it is safe on
		// the normal path too: it also covers a browser that hung on shutdown.
		h.Kill()
	}
}

// CloseAllBrowsers kills every browser that is still registered and returns
// how many were closed. Safe to call when nothing is running, and safe to call
// from another goroutine (the dashboard's Stop button does exactly that).
func CloseAllBrowsers() int {
	browsers.mu.Lock()
	handles := make([]browserHandle, 0, len(browsers.open))
	for id, h := range browsers.open {
		handles = append(handles, h)
		delete(browsers.open, id)
	}
	browsers.mu.Unlock()
	for _, h := range handles {
		h.Kill()
	}
	return len(handles)
}

// OpenBrowserCount reports how many browsers are registered right now.
func OpenBrowserCount() int {
	browsers.mu.Lock()
	defer browsers.mu.Unlock()
	return len(browsers.open)
}
