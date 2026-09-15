package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

type RequestID struct{ Workspace, Document, Revision, Operation uint64 }
type Job func(context.Context) (any, error)
type Result struct {
	ID    RequestID
	Value any
	Err   error
}
type queued struct {
	ctx context.Context
	id  RequestID
	job Job
}
type Pool struct {
	jobs    chan queued
	results chan Result
	latest  sync.Map
	closed  atomic.Bool
	wg      sync.WaitGroup
}

func NewPool(workers, queue int) *Pool {
	if workers < 1 {
		workers = 1
	}
	if queue < 1 {
		queue = 1
	}
	p := &Pool{jobs: make(chan queued, queue), results: make(chan Result, queue)}
	p.wg.Add(workers)
	for range workers {
		go p.work()
	}
	return p
}
func key(id RequestID) [2]uint64 { return [2]uint64{id.Workspace, id.Document} }
func (p *Pool) Submit(ctx context.Context, id RequestID, job Job) error {
	if p.closed.Load() {
		return errors.New("worker pool is closed")
	}
	p.latest.Store(key(id), id.Revision)
	select {
	case p.jobs <- queued{ctx, id, job}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *Pool) Results() <-chan Result { return p.results }
func (p *Pool) work() {
	defer p.wg.Done()
	for q := range p.jobs {
		value, err := q.job(q.ctx)
		if q.ctx.Err() != nil {
			continue
		}
		latest, _ := p.latest.Load(key(q.id))
		if latest.(uint64) != q.id.Revision {
			continue
		}
		select {
		case p.results <- Result{q.id, value, err}:
		case <-q.ctx.Done():
		}
	}
}
func (p *Pool) Close() {
	if p.closed.CompareAndSwap(false, true) {
		close(p.jobs)
		p.wg.Wait()
		close(p.results)
	}
}
