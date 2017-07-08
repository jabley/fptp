package fptp

import "io"

// SearchRequest is the query object for a Searcher
type SearchRequest struct {
}

// NewSearchRequest creates a new SearchRequest that should be treated as immutable.
func NewSearchRequest() *SearchRequest {
	return &SearchRequest{}
}

// Searcher defines the Search operation
type Searcher interface {
	Search(*SearchRequest) (io.Closer, error)
}
