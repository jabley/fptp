package fptp_test

import (
	"testing"
	"time"

	"io"

	"context"

	"github.com/jabley/fptp"
)

func TestUnusedCounterHasZeroUnclosed(t *testing.T) {
	counter := &CounterCloser{}
	if counter.Unclosed() != 0 {
		t.Fatalf("Unused counter should have zero unclosed")
	}
}

func TestUnclosedSearcherIsCounted(t *testing.T) {
	counter := &CounterCloser{}
	searcher := NewCountingSearcher(counter)

	closer, _ := searcher.Search(fptp.NewSearchRequest())
	defer closer.Close()
	if counter.Unclosed() != 1 {
		t.Fatalf("Unclosed Closer should have been counted")
	}
}

func TestCompositeWillTimeout(t *testing.T) {
	counter := &CounterCloser{}
	searcher := &delayedSearcher{
		delegate: NewCountingSearcher(counter),
		delay:    20 * time.Millisecond,
	}

	composite := fptp.NewCompositeSearcher([]fptp.Searcher{searcher}, 10*time.Millisecond)

	result, err := composite.Search(fptp.NewSearchRequest())

	if result != nil {
		defer result.Close()
		t.Fatalf("Expected nil result, but got %v", result)
	}

	if err == nil {
		t.Fatalf("Expected to get a timeout error but got nil")
	}

	if err != context.DeadlineExceeded {
		t.Fatalf("Got error %v", err)
	}
}

func TestWillWaitUntilSuccessIsReturned(t *testing.T) {
	counter := &CounterCloser{}
	searcher := &delayedSearcher{
		delegate: NewCountingSearcher(counter),
		delay:    10 * time.Millisecond,
	}

	// Searchers such that the erroring one should complete faster than the other one
	searchers := []fptp.Searcher{
		&erroringSearcher{
			err: io.EOF,
		},
		searcher,
	}
	composite := fptp.NewCompositeSearcher(searchers, 5*time.Second)

	result, err := composite.Search(fptp.NewSearchRequest())

	if err != nil {
		t.Fatalf("Expected that err should be nil, but got %v", err)
	}

	if result == nil {
		t.Fatalf("Expected non-nil result")
	}

	result.Close()
}

func TestWillTryAllSearchersAndReturnTheFailure(t *testing.T) {
	var searchers []fptp.Searcher
	for i := 0; i < 20; i++ {
		searchers = append(searchers, &erroringSearcher{err: io.EOF})
	}

	composite := fptp.NewCompositeSearcher(searchers, 5*time.Second)

	result, err := composite.Search(fptp.NewSearchRequest())

	if result != nil {
		defer result.Close()
		t.Fatalf("Expected nil result, but got %v", result)
	}

	if err == nil {
		t.Fatalf("Expected to get an error but got nil")
	}

	unusedSearchers := 0
	for i := 0; i < len(searchers); i++ {
		es := searchers[i].(*erroringSearcher)
		if !es.used {
			unusedSearchers++
		}
	}

	if unusedSearchers != 0 {
		t.Fatalf("Expected that all of the searchers had been used, but %d were not", unusedSearchers)
	}
}

func TestClosersAreNotLeaked(t *testing.T) {
	counter := &CounterCloser{}
	var searchers []fptp.Searcher
	for i := 0; i < 20; i++ {
		searchers = append(searchers, NewCountingSearcher(counter))
	}

	composite := fptp.NewCompositeSearcher(searchers, 5*time.Second)

	for i := 0; i < 100000; i++ {
		winner, _ := composite.Search(fptp.NewSearchRequest())
		winner.Close()
		assertClosersAllClosed(t, counter)
	}
}

func assertClosersAllClosed(t *testing.T, counter *CounterCloser) {
	backoff := 100 * time.Microsecond
	for {
		unclosed := counter.Unclosed()
		if unclosed == 0 {
			break
		}
		// fail if we've exceeded our time budgete - we don't want to spin forever
		if backoff > 10*time.Millisecond {
			t.Fatalf("Expected all closers to have been closed, but had %d still open after backing off to %v", unclosed, backoff)
		}

		// Give it a little while for all of the laggards to be closed
		time.Sleep(backoff)
		backoff = backoff * 2
	}
}

type delayedSearcher struct {
	delay    time.Duration
	delegate fptp.Searcher
}

func (ds *delayedSearcher) Search(req *fptp.SearchRequest) (io.Closer, error) {
	time.Sleep(ds.delay)
	return ds.delegate.Search(req)
}

type erroringSearcher struct {
	used bool
	err  error
}

func (es *erroringSearcher) Search(req *fptp.SearchRequest) (io.Closer, error) {
	es.used = true
	return nil, es.err
}
