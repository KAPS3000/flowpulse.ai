package transport

import (
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// BackpressureController manages flow control between agent and aggregator.
// When the aggregator is overloaded, it signals the agent to slow down.
type BackpressureController struct {
	// Current state
	sendRate       atomic.Int64  // messages per second allowed
	queueDepth     atomic.Int64  // current queue depth
	dropped        atomic.Int64  // total dropped messages
	backpressured  atomic.Bool

	// Config
	maxQueueDepth  int64
	minRate        int64
	maxRate        int64
	rampUpStep     int64
	rampDownFactor int64
}

func NewBackpressureController(maxQueue, minRate, maxRate int64) *BackpressureController {
	bp := &BackpressureController{
		maxQueueDepth:  maxQueue,
		minRate:        minRate,
		maxRate:        maxRate,
		rampUpStep:     100,
		rampDownFactor: 2,
	}
	bp.sendRate.Store(maxRate)
	return bp
}

// ShouldSend returns true if the message should be sent, false if it should be dropped.
func (bp *BackpressureController) ShouldSend() bool {
	depth := bp.queueDepth.Load()
	if depth > bp.maxQueueDepth {
		bp.backpressured.Store(true)
		bp.dropped.Add(1)
		return false
	}

	// Adaptive rate: reduce if queue is >75% full
	threshold := bp.maxQueueDepth * 3 / 4
	if depth > threshold {
		currentRate := bp.sendRate.Load()
		newRate := currentRate / bp.rampDownFactor
		if newRate < bp.minRate {
			newRate = bp.minRate
		}
		bp.sendRate.Store(newRate)
		bp.backpressured.Store(true)
	}

	return true
}

// Enqueue increments the queue depth counter.
func (bp *BackpressureController) Enqueue() {
	bp.queueDepth.Add(1)
}

// Dequeue decrements the queue depth counter after successful send.
func (bp *BackpressureController) Dequeue() {
	bp.queueDepth.Add(-1)

	// If queue is draining, ramp up the rate
	depth := bp.queueDepth.Load()
	if depth < bp.maxQueueDepth/4 {
		currentRate := bp.sendRate.Load()
		newRate := currentRate + bp.rampUpStep
		if newRate > bp.maxRate {
			newRate = bp.maxRate
		}
		bp.sendRate.Store(newRate)
		bp.backpressured.Store(false)
	}
}

// Stats returns current backpressure statistics.
func (bp *BackpressureController) Stats() BackpressureStats {
	return BackpressureStats{
		QueueDepth:    bp.queueDepth.Load(),
		MaxQueue:      bp.maxQueueDepth,
		CurrentRate:   bp.sendRate.Load(),
		TotalDropped:  bp.dropped.Load(),
		Backpressured: bp.backpressured.Load(),
	}
}

type BackpressureStats struct {
	QueueDepth    int64 `json:"queue_depth"`
	MaxQueue      int64 `json:"max_queue"`
	CurrentRate   int64 `json:"current_rate"`
	TotalDropped  int64 `json:"total_dropped"`
	Backpressured bool  `json:"backpressured"`
}

// Monitor runs a background loop that logs backpressure state periodically.
func (bp *BackpressureController) Monitor(done <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			stats := bp.Stats()
			if stats.Backpressured || stats.TotalDropped > 0 {
				log.Warn().
					Int64("queue_depth", stats.QueueDepth).
					Int64("rate", stats.CurrentRate).
					Int64("dropped", stats.TotalDropped).
					Bool("backpressured", stats.Backpressured).
					Msg("backpressure status")
			}
		}
	}
}
