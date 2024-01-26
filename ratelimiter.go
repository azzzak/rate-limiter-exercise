package ratelimiter

import (
	"context"
	"errors"
	"time"
)

// ErrBlocked reports if service is blocked.
var ErrBlocked = errors.New("blocked")

// Service defines external service that can process batches of items.
type Service interface {
	GetLimits() (n int64, p time.Duration)
	Process(ctx context.Context, batch Batch) error
}

// Batch is a batch of items.
type Batch []Item

// Item is some abstract item.
type Item struct{}

type RateLimiter struct {
	batchSize     int
	limitNumber   int64
	limitInterval time.Duration

	workDelay time.Duration
	minDelay  time.Duration
	c         chan Item
	errChan   chan error

	buffer    Batch
	itemsLeft int64
	timeLeft  time.Duration
}

func NewRateLimiter(batchSize int, limitNumber int64, limitInterval, minDelay time.Duration) (*RateLimiter, chan Item, chan error) {
	var min time.Duration
	if minDelay > 0 {
		min = minDelay
	}

	c := make(chan Item, 1)
	errChan := make(chan error, 1)

	return &RateLimiter{
		batchSize:     batchSize,
		limitNumber:   limitNumber,
		limitInterval: limitInterval,

		workDelay: calcInterval(batchSize, limitNumber, limitInterval),
		minDelay:  min,
		c:         c,
		errChan:   errChan,

		buffer:    make(Batch, 0, batchSize),
		itemsLeft: limitNumber,
		timeLeft:  limitInterval,
	}, c, errChan
}

func (r *RateLimiter) GetLimits() (n int64, p time.Duration) {
	return r.itemsLeft, r.timeLeft
}

func (r *RateLimiter) Process(ctx context.Context, batch Batch) error {
	return nil
}

func calcInterval(bacthSize int, n int64, p time.Duration) time.Duration {
	return time.Duration(int64(p) / n * int64(bacthSize))
}

func (r *RateLimiter) updateWorkDelay() time.Duration {
	numLeft, timeLeft := r.GetLimits()
	newInterval := calcInterval(r.batchSize, numLeft, timeLeft)

	if newInterval < r.minDelay {
		newInterval = r.minDelay
	}

	r.workDelay = newInterval
	return newInterval
}

// preparing copy of buffer to avoid possible threading problems
func (r *RateLimiter) batchToProcess() Batch {
	var temp Batch
	numLeft := r.itemsLeft

	// if batch exceeds limits some elements must remain in buffer
	if int64(len(r.buffer)) > numLeft {
		temp = make(Batch, numLeft)
		copy(temp, r.buffer)

		copy(r.buffer, r.buffer[numLeft:])
		remIndex := int64(r.batchSize) - numLeft
		r.buffer = r.buffer[:remIndex]

		return temp
	}

	temp = make(Batch, len(r.buffer))
	copy(temp, r.buffer)
	r.buffer = r.buffer[:0]

	return temp
}

func (r *RateLimiter) Run(ctx context.Context) {
	ticker := time.NewTicker(r.workDelay)

	for {
		if r.timeLeft <= 0 {
			r.timeLeft = r.limitInterval
			r.itemsLeft = r.limitNumber
		}

		ticker.Reset(r.updateWorkDelay())

		ts := time.Now()

		select {
		case <-ticker.C:
			doneNumber := len(r.buffer)
			if len(r.buffer) > 0 {
				if err := r.Process(ctx, r.batchToProcess()); err != nil {
					r.errChan <- err
				}
				r.itemsLeft -= int64(doneNumber)
			}

			r.timeLeft -= time.Since(ts)
		case item := <-r.c:
			r.buffer = append(r.buffer, item)
			if len(r.buffer) < r.batchSize {
				r.timeLeft += time.Since(ts)
				continue
			}

			doneNumber := len(r.buffer)
			if err := r.Process(ctx, r.batchToProcess()); err != nil {
				r.errChan <- err
			}

			r.itemsLeft -= int64(doneNumber)

			// wait ticker
			<-ticker.C

			r.timeLeft -= time.Since(ts)
		case <-ctx.Done():
			if err := r.Process(ctx, r.buffer); err != nil {
				r.errChan <- err
			}

			ticker.Stop()
			return
		}
	}
}
