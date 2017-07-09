package fptp_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"io"

	"context"

	"github.com/jabley/fptp"
)

func TestUnusedCounterHasZeroUnclosed(t *testing.T) {
	counter := &CounterCloser{}
	assertEqual(t, 0, counter.Unclosed(), "Unused counter should have zero unclosed")
}

func TestUnclosedSearcherIsCounted(t *testing.T) {
	counter := &CounterCloser{}
	searcher := NewCountingSearcher(counter)

	closer, _ := searcher.Search(fptp.NewSearchRequest())
	defer closer.Close()
	assertEqual(t, 1, counter.Unclosed(), "Unclosed Closer should have been counted")
}

func TestCloserCanOnlyBeClosedOnce(t *testing.T) {
	counter := &CounterCloser{}

	assertEqual(t, 0, counter.Unclosed(), "Unused counter should have zero unclosed")
	closer := counter.NewCloser()
	assertEqual(t, 1, counter.Unclosed(), "Counter should have a single closer outstanding")
	closer.Close()
	assertEqual(t, 0, counter.Unclosed(), "Closer has now been closed")
	closer.Close()
	closer.Close()
	assertEqual(t, 0, counter.Unclosed(), "Repeatedly closing has no effect")
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
		assertEqual(t, nil, result, "Expected result to be nil")
	}

	assertEqual(t, context.DeadlineExceeded, err, "Expected to get a timeout error")
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

	if result != nil {
		defer result.Close()
	}
	assertEqual(t, nil, err, "Expected err to be nil")
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
		assertEqual(t, nil, result, "Expected result to be nil")
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

	assertEqual(t, 0, unusedSearchers, "Expected that all of the searchers had been used, but %d were not", unusedSearchers)
}

func TestClosersAreNotLeaked(t *testing.T) {
	counter := &CounterCloser{}
	var searchers []fptp.Searcher
	for i := 0; i < 20; i++ {
		searchers = append(searchers, NewCountingSearcher(counter))
	}

	composite := fptp.NewCompositeSearcher(searchers, 5*time.Second)

	for i := 0; i < 10000; i++ {
		winner, _ := composite.Search(fptp.NewSearchRequest())
		winner.Close()
		assertClosersAllClosed(t, counter)
	}
}

func assertEqual(t *testing.T, expected interface{}, actual interface{}, msgAndArgs ...interface{}) {
	if expected == nil || actual == nil {
		if actual == expected {
			return
		}
		fail(t, expected, actual, msgAndArgs...)
	}

	if !reflect.DeepEqual(expected, actual) {
		fail(t, expected, actual, msgAndArgs...)
	}
}

func fail(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Fatalf("\nExpected:\t%#v\nActual:\t\t%#v\n%s", expected, actual, messageFromMsgAndArgs(msgAndArgs...))
}

func messageFromMsgAndArgs(msgAndArgs ...interface{}) string {
	if len(msgAndArgs) == 0 || msgAndArgs == nil {
		return ""
	}
	if len(msgAndArgs) == 1 {
		return msgAndArgs[0].(string)
	}
	if len(msgAndArgs) > 1 {
		return fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...)
	}
	return ""
}

func assertClosersAllClosed(t *testing.T, counter *CounterCloser) {
	backoff := 1 * time.Nanosecond
	for {
		unclosed := counter.Unclosed()
		if unclosed == 0 {
			break
		}
		// fail if we've exceeded our time budgete - we don't want to spin forever
		if backoff > 10*time.Millisecond {
			assertEqual(t, 0, unclosed, "Expected all closers to have been closed, but had %d still open after backing off to %v", unclosed, backoff)
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
