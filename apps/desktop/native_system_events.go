package main

import (
	"context"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// nativeSystemEventPump keeps OS callbacks non-blocking and serializes changes
// before they enter the core recovery gate. A capacity of one coalesces bursts;
// the core performs the final 250 ms debounce.
type nativeSystemEventPump struct {
	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	events chan string
	stops  []func()
	closed bool
}

func newNativeSystemEventPump(parent context.Context, deliver func(string)) *nativeSystemEventPump {
	ctx, cancel := context.WithCancel(parent)
	p := &nativeSystemEventPump{cancel: cancel, done: make(chan struct{}), events: make(chan string, 1)}
	go func() {
		defer close(p.done)
		for {
			select {
			case <-ctx.Done():
				return
			case reason := <-p.events:
				deliver(reason)
			}
		}
	}()
	return p
}

func (p *nativeSystemEventPump) notify(reason string) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	select {
	case p.events <- reason:
	default:
	}
	p.mu.Unlock()
}

func (p *nativeSystemEventPump) addStop(stop func()) {
	if stop == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		stop()
		return
	}
	p.stops = append(p.stops, stop)
	p.mu.Unlock()
}

func (p *nativeSystemEventPump) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	stops := append([]func(){}, p.stops...)
	p.stops = nil
	p.mu.Unlock()
	for i := len(stops) - 1; i >= 0; i-- {
		stops[i]()
	}
	p.cancel()
	<-p.done
}

func (a *App) startNativeSystemEvents() {
	if a.nativeEvents != nil || a.ctx == nil || a.core == nil {
		return
	}
	pump := newNativeSystemEventPump(a.ctx, func(reason string) {
		if err := a.core.NetworkChanged(reason); err != nil {
			slog.Warn("native network recovery event failed", "reason", reason, "error", err)
		}
	})
	a.nativeEvents = pump
	if a.runtimeApp != nil {
		pump.addStop(a.runtimeApp.Event.OnApplicationEvent(events.Common.SystemWillSleep, func(*application.ApplicationEvent) {
			pump.notify("sleep")
		}))
		pump.addStop(a.runtimeApp.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) {
			pump.notify("wake")
		}))
	}
	stop, err := startNativeNetworkMonitor(func() { pump.notify("network") })
	if err != nil {
		slog.Warn("native network change monitor unavailable; snapshot fallback remains active", "error", err)
		return
	}
	pump.addStop(stop)
}

func (a *App) stopNativeSystemEvents() {
	pump := a.nativeEvents
	a.nativeEvents = nil
	if pump != nil {
		pump.close()
	}
}
