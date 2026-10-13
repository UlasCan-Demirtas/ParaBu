package worker

import (
	"context"
	"sync"
)

// Job represents a single unit of work (e.g. sending a webhook).
type Job func(context.Context)

// Pool is a bounded goroutine pool.
type Pool struct {
	concurrency int
	jobs        chan Job
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewPool creates a new bounded worker pool.
func NewPool(concurrency int) *Pool {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Pool{
		concurrency: concurrency,
		jobs:        make(chan Job, concurrency*2),
		ctx:         ctx,
		cancel:      cancel,
	}

	p.start()
	return p
}

func (p *Pool) start() {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-p.ctx.Done():
					return
				case job, ok := <-p.jobs:
					if !ok {
						return
					}
					job(p.ctx)
				}
			}
		}()
	}
}

// Submit enqueues a job. It blocks if the queue is full.
func (p *Pool) Submit(job Job) {
	select {
	case <-p.ctx.Done():
		return
	case p.jobs <- job:
	}
}

// Stop gracefully shuts down the worker pool, waiting for in-flight jobs to finish.
func (p *Pool) Stop() {
	p.cancel()
	close(p.jobs)
	p.wg.Wait()
}
