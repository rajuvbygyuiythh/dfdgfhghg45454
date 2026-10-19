package main

import (
	"sync"
	"time"
)

// Background Bot live log: same bounded feed model as Show Mode but for API workers.
// Separate from logs.txt and from Show Mode feed so each tab keeps its own history.
// Entries are memory-only and never include credentials.
const botLogCapacity = 500

type BotLogEntry struct {
	Seq       uint64 `json:"seq"`
	Time      string `json:"time"`
	Kind      string `json:"kind"` // in | out | skip | system | error
	Text      string `json:"text,omitempty"`
	Rule      string `json:"rule,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Level     string `json:"level,omitempty"` // info | warn | error
	Profile   string `json:"profile,omitempty"`
	SettingOK bool   `json:"setting_ok"`
}

type BotLogSnapshot struct {
	Version uint64       `json:"version"`
	Entries []BotLogEntry `json:"entries"`
	Dropped int          `json:"dropped"`
}

type botLogFeed struct {
	mu      sync.Mutex
	seq     uint64
	dropped int
	entries []BotLogEntry
}

func (f *botLogFeed) add(entry BotLogEntry) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	entry.Seq = f.seq
	if entry.Time == "" {
		entry.Time = time.Now().Format("15:04:05")
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	f.entries = append(f.entries, entry)
	if len(f.entries) > botLogCapacity {
		drop := len(f.entries) - botLogCapacity
		f.entries = f.entries[drop:]
		f.dropped += drop
	}
	return f.seq
}

func (f *botLogFeed) snapshot(after uint64) BotLogSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]BotLogEntry, 0, len(f.entries))
	for _, entry := range f.entries {
		if entry.Seq > after {
			out = append(out, entry)
		}
	}
	return BotLogSnapshot{Version: f.seq, Entries: out, Dropped: f.dropped}
}

func (f *botLogFeed) clear() uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = nil
	f.dropped = 0
	return f.seq
}

func (a *App) botLogFeed() *botLogFeed {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.botLog == nil {
		a.botLog = &botLogFeed{}
	}
	return a.botLog
}

func (a *App) addBotLog(entry BotLogEntry) {
	feed := a.botLogFeed()
	seq := feed.add(entry)
	a.emit("ui:botlog", map[string]uint64{"version": seq})
}

func (a *App) botEvent(kind, text, rule, detail, profile string, settingOK bool) {
	level := "info"
	if kind == "error" {
		level = "error"
	} else if kind == "skip" {
		level = "warn"
	}
	a.addBotLog(BotLogEntry{Kind: kind, Text: text, Rule: rule, Detail: detail, Level: level, Profile: profile, SettingOK: settingOK})
}

// GetBotLog returns entries after the given sequence number. The
// frontend polls this once per second and also refreshes on ui:botlog.
func (a *App) GetBotLog(after uint64) BotLogSnapshot {
	return a.botLogFeed().snapshot(after)
}

// ClearBotLog clears the in-memory feed; logs.txt keeps its history.
func (a *App) ClearBotLog() error {
	feed := a.botLogFeed()
	version := feed.clear()
	a.emit("ui:botlog", map[string]uint64{"version": version})
	return nil
}
