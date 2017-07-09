package fptp_test

import (
	"io"
	"sync"

	"github.com/jabley/fptp"
)

type closeObserver interface {
	Closed()
}

type closeDispenser interface {
	NewCloser() io.Closer
}

type CounterCloser struct {
	mu            sync.Mutex
	awaitingClose int
}

func (c *CounterCloser) NewCloser() io.Closer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.awaitingClose++
	return &countingCloser{closeObserver: c}
}

func (c *CounterCloser) Closed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.awaitingClose--
}

func (c *CounterCloser) Unclosed() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.awaitingClose
}

type countingCloser struct {
	mu            sync.Mutex
	closed        bool
	closeObserver closeObserver
}

func (cc *countingCloser) Close() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if !cc.closed {
		cc.closed = true
		cc.closeObserver.Closed()
	}
	return nil
}

// NewCountingSearcher returns a Searcher that uses the provided CounterCloser to generate responses.
func NewCountingSearcher(dispenser *CounterCloser) fptp.Searcher {
	return &basicSearcher{dispenser: dispenser}
}

type basicSearcher struct {
	dispenser *CounterCloser
}

func (bs *basicSearcher) Search(req *fptp.SearchRequest) (io.Closer, error) {
	return bs.dispenser.NewCloser(), nil
}
