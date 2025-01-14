package worker

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

var (
	ErrProcessStopped   = fmt.Errorf("worker process has stopped")
	ErrContextCancelled = fmt.Errorf("worker context cancelled")
)

const (
	// DefaultMaxRetryCount is the number of times a job will be retried before being dropped.
	DefaultMaxRetryCount = 6
	// DefaultNotifyRetryDepth is the retry queue depth at which the worker group will log a warning.
	DefaultNotifyRetryDepth = 200
	// DefaultNotifyQueueDepth is the queue depth at which the worker group will log a warning.
	DefaultNotifyQueueDepth = 100
	// DefaultWorkerCount is the default number of workers in a Group.
	DefaultWorkerCount = 10
)

type worker struct {
	Name  string
	Queue chan *worker
	Retry chan Job
	Lggr  logger.Logger
}

func (w *worker) Do(ctx context.Context, job Job) {
	if ctx.Err() == nil {
		start := time.Now()
		w.Lggr.Debugf("Starting job %s", job.String())
		if err := job.Run(ctx); err != nil {
			w.Lggr.Errorf("job %s failed with error; retrying: %s", job, err)
			w.Retry <- job
		}
		// TODO: add prom metric
		w.Lggr.Debugf("Finished job %s in %s", job.String(), time.Since(start))
	}

	// put itself back on the queue when done
	select {
	case w.Queue <- w:
	default:
	}
}

type Group struct {
	// service state management
	services.Service
	engine *services.Engine

	// dependencies and configuration
	maxWorkers    int
	maxRetryCount uint8
	lggr          logger.SugaredLogger

	// worker group state
	workers       chan *worker
	queue         *queue[Job]
	input         chan Job
	chInputNotify chan struct{}

	chStopInputs chan struct{}
	queueClosed  atomic.Bool

	// retry queue
	chRetry  chan Job
	mu       sync.RWMutex
	retryMap map[string]retryableJob
}

func NewGroup(workers int, lggr logger.SugaredLogger) *Group {
	g := &Group{
		maxWorkers:    workers,
		maxRetryCount: DefaultMaxRetryCount,
		workers:       make(chan *worker, workers),
		lggr:          lggr,
		queue:         newQueue[Job](0),
		input:         make(chan Job, 1),
		chInputNotify: make(chan struct{}, 1),
		chStopInputs:  make(chan struct{}),
		chRetry:       make(chan Job, 1),
		retryMap:      make(map[string]retryableJob),
	}

	g.Service, g.engine = services.Config{
		Name:  "WorkerGroup",
		Start: g.start,
		Close: g.close,
	}.NewServiceEngine(lggr)

	for idx := range workers {
		g.workers <- &worker{
			Name:  fmt.Sprintf("worker-%d", idx+1),
			Queue: g.workers,
			Retry: g.chRetry,
			Lggr:  g.lggr,
		}
	}

	return g
}

var _ services.Service = &Group{}

func (g *Group) start(ctx context.Context) error {
	g.engine.Go(g.runQueuing)
	g.engine.Go(g.runProcessing)
	g.engine.Go(g.runRetryQueue)
	g.engine.Go(g.runRetries)

	return nil
}

func (g *Group) close() error {
	if !g.queueClosed.Load() {
		g.queueClosed.Store(true)
		close(g.chStopInputs)
	}

	return nil
}

// Do adds a new work item onto the work queue. This function blocks until
// the work queue clears up or the context is cancelled. This allows a max wait
// time for the queue to open. Or a context can wrap a collection of jobs that
// need to be run and when the context cancels, the jobs don't get added to the
// queue.
func (g *Group) Do(ctx context.Context, job Job) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w; work not added to queue", ErrContextCancelled)
	}

	if g.queueClosed.Load() {
		return fmt.Errorf("%w; work not added to queue", ErrProcessStopped)
	}

	select {
	case g.input <- job:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w; work not added to queue", ErrContextCancelled)
	case <-g.chStopInputs:
		return fmt.Errorf("%w; work not added to queue", ErrProcessStopped)
	}
}

func (g *Group) runQueuing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-g.input:
			g.queue.Add(item)

			// notify that new work item came in
			// drop if notification channel is full
			select {
			case g.chInputNotify <- struct{}{}:
			default:
			}
		}
	}
}

func (g *Group) runProcessing(ctx context.Context) {
Loop:
	for {
		select {
		// watch notification channel and begin processing queue
		// when notification occurs
		case <-g.chInputNotify:
			g.processQueue(ctx)
		case <-ctx.Done():
			break Loop
		}
	}
}

func (g *Group) runRetryQueue(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-g.chRetry:
			var retry retryableJob

			switch typedJob := job.(type) {
			case retryableJob:
				retry = typedJob
				retry.count++

				if retry.count > g.maxRetryCount {
					g.lggr.Criticalf("job %s exceeded max retries %d/%d", job, retry.count, g.maxRetryCount)
				}

				wait := calculateExponentialBackoff(min(retry.count, g.maxRetryCount))
				g.lggr.Errorf("retrying job in %dms", wait/time.Millisecond)

				retry.when = time.Now().Add(wait)
			default:
				wait := calculateExponentialBackoff(0)

				g.lggr.Errorf("retrying job %s in %s", job, wait)

				retry = retryableJob{
					name: createRandomString(12),
					job:  job,
					when: time.Now().Add(wait),
				}
			}

			g.mu.Lock()
			g.retryMap[retry.name] = retry

			if len(g.retryMap) >= DefaultNotifyRetryDepth {
				g.lggr.Errorf("retry queue depth: %d", len(g.retryMap))
			}
			g.mu.Unlock()
		}
	}
}

func (g *Group) runRetries(ctx context.Context) {
	for {
		// run timer on minimum backoff
		timer := time.NewTimer(calculateExponentialBackoff(0))

		select {
		case <-ctx.Done():
			timer.Stop()

			return
		case <-timer.C:
			g.mu.RLock()
			keys := make([]string, 0, len(g.retryMap))
			retries := make([]retryableJob, 0, len(g.retryMap))

			for key, retry := range g.retryMap {
				if time.Now().After(retry.when) {
					keys = append(keys, key)
					retries = append(retries, retry)
				}
			}
			g.mu.RUnlock()

			for idx, key := range keys {
				g.mu.Lock()
				delete(g.retryMap, key)
				g.mu.Unlock()

				g.doJob(ctx, retries[idx])
			}

			timer.Stop()
		}
	}
}

func (g *Group) processQueue(ctx context.Context) {
	for {
		if g.queue.Len() == 0 {
			break
		}

		if g.queue.Len() >= DefaultNotifyQueueDepth {
			g.lggr.Errorf("queue depth: %d", g.queue.Len())
		}

		value, err := g.queue.Pop()

		// an error from pop means there is nothing to pop
		// the length check above should protect from that, but just in case
		// this error also breaks the loop
		if err != nil {
			break
		}

		g.doJob(ctx, value)
	}
}

func (g *Group) doJob(ctx context.Context, job Job) {
	wkr := <-g.workers

	go wkr.Do(ctx, job)
}

type queue[T any] struct {
	mu     sync.RWMutex
	values []T
}

func newQueue[T any](maxLen uint) *queue[T] {
	values := make([]T, maxLen)

	return &queue[T]{
		values: values,
	}
}

func (q *queue[T]) Add(values ...T) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.values = append(q.values, values...)
}

func (q *queue[T]) Pop() (T, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.values) == 0 {
		return getZero[T](), fmt.Errorf("no values to return")
	}

	val := q.values[0]

	if len(q.values) > 1 {
		q.values = q.values[1:]
	} else {
		q.values = []T{}
	}

	return val, nil
}

func (q *queue[T]) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.values)
}

func getZero[T any]() T {
	var result T
	return result
}

func createRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)

	for i := range b {
		rVal, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			rVal = big.NewInt(12)
		}

		b[i] = charset[rVal.Int64()]
	}

	return string(b)
}

func calculateExponentialBackoff(retries uint8) time.Duration {
	// 200ms, 400ms, 800ms, 1.6s, 3.2s, 6.4s
	return time.Duration(2<<retries) * 100 * time.Millisecond
}
