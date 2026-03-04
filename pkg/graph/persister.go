// Copyright (c) 2026 haitch
// Licensed under the Apache License, Version 2.0
// https://www.apache.org/licenses/LICENSE-2.0

package graph

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// AdaptivePersister handles graph persistence with adaptive timing.
// Under high write load, saves are batched to reduce I/O.
// Under low load, changes persist quickly.
type AdaptivePersister struct {
	graph    Graph
	filename string
	logger   zerolog.Logger

	// Adaptive state
	activeWriters atomic.Int64
	dirty         atomic.Bool
	lastSave      time.Time
	mu            sync.Mutex

	// Configuration
	baseInterval time.Duration // minimum interval between saves
	maxInterval  time.Duration // maximum interval between saves

	// Lifecycle
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewAdaptivePersister creates a new adaptive persister
func NewAdaptivePersister(g Graph, filename string, logger zerolog.Logger) *AdaptivePersister {
	return &AdaptivePersister{
		graph:        g,
		filename:     filename,
		logger:       logger,
		baseInterval: 500 * time.Millisecond,
		maxInterval:  30 * time.Second,
		lastSave:     time.Now(),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins the background persistence loop
func (p *AdaptivePersister) Start() {
	go p.loop()
	p.logger.Info().
		Dur("base_interval", p.baseInterval).
		Dur("max_interval", p.maxInterval).
		Msg("Adaptive graph persister started")
}

// Stop gracefully shuts down, ensuring final save
func (p *AdaptivePersister) Stop() {
	close(p.stopCh)
	<-p.doneCh
	p.logger.Info().Msg("Adaptive graph persister stopped")
}

// MarkDirty signals that the graph has been modified.
// Call this after UpdateFromEntity.
func (p *AdaptivePersister) MarkDirty() {
	p.dirty.Store(true)
}

// WriterEnter increments active writer count.
// Call at the start of a write operation.
func (p *AdaptivePersister) WriterEnter() {
	p.activeWriters.Add(1)
}

// WriterExit decrements active writer count.
// Call at the end of a write operation (defer recommended).
func (p *AdaptivePersister) WriterExit() {
	p.activeWriters.Add(-1)
}

// currentInterval calculates the adaptive save interval.
// Uses logarithmic scaling: more writers → longer intervals.
//
// Scaling table:
//   1 writer   → base * 1.0 = 500ms
//   3 writers  → base * 2.1 = 1.05s
//   10 writers → base * 3.3 = 1.65s
//   50 writers → base * 4.9 = 2.45s
//   100 writers → base * 5.6 = 2.8s
func (p *AdaptivePersister) currentInterval() time.Duration {
	writers := p.activeWriters.Load()
	if writers <= 0 {
		writers = 1
	}

	// interval = base * (1 + ln(writers))
	multiplier := 1.0 + math.Log(float64(writers))
	interval := time.Duration(float64(p.baseInterval) * multiplier)

	if interval > p.maxInterval {
		interval = p.maxInterval
	}

	return interval
}

// Stats returns current persister statistics
func (p *AdaptivePersister) Stats() map[string]interface{} {
	return map[string]interface{}{
		"active_writers":   p.activeWriters.Load(),
		"dirty":            p.dirty.Load(),
		"current_interval": p.currentInterval().String(),
		"last_save":        p.lastSave.Format(time.RFC3339),
	}
}

func (p *AdaptivePersister) loop() {
	defer close(p.doneCh)

	ticker := time.NewTicker(100 * time.Millisecond) // check frequently
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			// Final save on shutdown
			p.save("shutdown")
			return

		case <-ticker.C:
			if !p.dirty.Load() {
				continue
			}

			elapsed := time.Since(p.lastSave)
			required := p.currentInterval()

			if elapsed >= required {
				p.save("periodic")
			}
		}
	}
}

func (p *AdaptivePersister) save(reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.dirty.Load() {
		return // already saved by another goroutine
	}

	if err := p.graph.Save(p.filename); err != nil {
		p.logger.Error().Err(err).Str("reason", reason).Msg("Failed to save graph")
		return
	}

	p.dirty.Store(false)
	p.lastSave = time.Now()

	writers := p.activeWriters.Load()
	p.logger.Debug().
		Str("reason", reason).
		Int64("active_writers", writers).
		Dur("interval", p.currentInterval()).
		Msg("Graph saved")
}
