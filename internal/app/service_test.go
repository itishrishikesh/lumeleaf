package app

import (
	"context"
	"testing"
	"time"
)

func TestStaleResultDiscarded(t *testing.T) {
	p := NewPool(1, 4)
	defer p.Close()
	blocked := make(chan struct{})
	_ = p.Submit(context.Background(), RequestID{Workspace: 1, Document: 1, Revision: 1}, func(context.Context) (any, error) { <-blocked; return "old", nil })
	_ = p.Submit(context.Background(), RequestID{Workspace: 1, Document: 1, Revision: 2}, func(context.Context) (any, error) { return "new", nil })
	close(blocked)
	select {
	case got := <-p.Results():
		if got.Value != "new" {
			t.Fatalf("got %v", got.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}
func TestCancelledOperationsDoNotPublish(t *testing.T) {
	p := NewPool(4, 32)
	for i := 0; i < 10000; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = p.Submit(ctx, RequestID{Workspace: 1, Document: uint64(i), Revision: 1}, func(ctx context.Context) (any, error) { return nil, ctx.Err() })
	}
	p.Close()
	for range p.Results() {
		t.Fatal("cancelled result published")
	}
}
