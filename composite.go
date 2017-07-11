package fptp

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

var ErrAllSearchersFailed = errors.New("fptp: all searchers failed")

type compositeSearcher struct {
	searchers []Searcher
	timeout   time.Duration
}

// NewCompositeSearcher returns a Searcher that will try all of the provided Searchers
func NewCompositeSearcher(searchers []Searcher, timeout time.Duration) Searcher {
	return &compositeSearcher{
		searchers: searchers,
		timeout:   timeout,
	}
}

// searchResponse is a nicer way of packaging the result of Searcher.Search for
// use with channels.
type searchResponse struct {
	closer io.Closer
	err    error
}

func (c *compositeSearcher) Search(req *SearchRequest) (io.Closer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	// We use a WaitGroup to keep track of when we've tried all of the Searcher instances
	var wg sync.WaitGroup

	// Synchronise on the channel
	respc := make(chan searchResponse)

	// Make all of the requests
	for _, s := range c.searchers {
		wg.Add(1)
		go func(req *SearchRequest, s Searcher) {
			defer wg.Done()
			c.search(ctx, s, req, respc)
		}(req, s)
	}

	// Housekeeping to track when we've tried all of the Searchers
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	return c.waitForSearchCompletion(ctx, respc, done)
}

func (c *compositeSearcher) search(ctx context.Context, s Searcher, req *SearchRequest, respc chan<- searchResponse) {

	res, err := s.Search(req)

	select {
	case <-ctx.Done():
		// Another Searcher already returned a successful response. Ensure we don't leak things
		res.Close()
	default:
		respc <- searchResponse{res, err}
	}
}

func (c *compositeSearcher) waitForSearchCompletion(ctx context.Context, respc chan searchResponse, done chan struct{}) (io.Closer, error) {
	var lastErr error

	for {
		select {
		case <-ctx.Done(): // timed out
			return nil, ctx.Err() // context.DeadlineExceeded
		case r := <-respc: // We got a response from one of the Searchers
			if r.err == nil { // success!
				return r.closer, r.err
			}
			lastErr = r.err // failed, keep track of why
		case <-done:
			// we've tried every Searcher, and none of them returned a successful response
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, ErrAllSearchersFailed
		}
	}
}
