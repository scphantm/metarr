package services

import (
	"errors"
	"fmt"
	"sort"

	"go.einride.tech/aip/ordering"
	"go.einride.tech/aip/pagination"
)

// The List helpers wrap go.einride.tech/aip so every collection service
// (SonarrInterfaceService, DirectoryScannerService, …) implements the
// AIP-158 / 132 / 160 List contract the same way: a page_size / page_token
// window over an in-memory slice, an order_by sort, and a filter entry point.
// The config collections are operator-bounded, so paging and sorting happen
// on the whole slice in memory rather than being pushed into storage.

const (
	// defaultListPageSize is returned when a request leaves page_size unset
	// or zero; maxListPageSize caps what a caller can ask for. Both match
	// AIP-158's guidance and are deliberately generous — config collections
	// are small.
	defaultListPageSize = 50
	maxListPageSize     = 1000
)

// errFilterUnsupported is returned by parseListFilter for any non-empty
// filter. AIP-160 filter-expression translation is deferred until a
// large-data service needs it (see AGENTS.md); until then only the empty
// filter is honoured and everything else is Unimplemented (see errors.go).
var errFilterUnsupported = errors.New("filter expressions are not supported yet")

// errBadPageToken wraps a page_token that did not come from a previous
// response for the same call. It maps to InvalidArgument (see errors.go).
var errBadPageToken = errors.New("invalid page_token")

// listPageToken is the opaque cursor the pagination helpers encode into
// page_token: a plain offset into the ordered slice.
type listPageToken struct {
	Offset int
}

// clampPageSize resolves a request's page_size to a concrete window size,
// applying the default and the ceiling.
func clampPageSize(pageSize int32) int {
	switch {
	case pageSize <= 0:
		return defaultListPageSize
	case pageSize > maxListPageSize:
		return maxListPageSize
	default:
		return int(pageSize)
	}
}

// paginateSlice returns the page_token-delimited window of items and the
// token for the next page ("" when the window reaches the end). A page_token
// that does not decode to a valid offset is errBadPageToken.
func paginateSlice[T any](items []T, pageSize int32, pageToken string) (page []T, nextPageToken string, err error) {
	offset := 0
	if pageToken != "" {
		var tok listPageToken
		if decodeErr := pagination.DecodePageTokenStruct(pageToken, &tok); decodeErr != nil || tok.Offset < 0 {
			return nil, "", fmt.Errorf("%w: %v", errBadPageToken, decodeErr)
		}
		offset = tok.Offset
	}
	if offset >= len(items) {
		return nil, "", nil
	}

	size := clampPageSize(pageSize)
	end := offset + size
	if end >= len(items) {
		return items[offset:], "", nil
	}
	return items[offset:end], pagination.EncodePageTokenStruct(listPageToken{Offset: end}), nil
}

// orderBySlice sorts items in place by an AIP-132 order_by string. less maps
// each sortable field name to a comparator (negative if a<b). An order_by
// naming a field absent from less is errUnknownPath (InvalidArgument); an
// empty order_by leaves items untouched.
func orderBySlice[T any](items []T, orderBy string, less map[string]func(a, b T) int) error {
	if orderBy == "" {
		return nil
	}
	var parsed ordering.OrderBy
	if err := parsed.UnmarshalString(orderBy); err != nil {
		return fmt.Errorf("%w: %v", errUnknownPath, err)
	}
	for _, f := range parsed.Fields {
		if _, ok := less[f.Path]; !ok {
			return fmt.Errorf("%w: order_by field %q", errUnknownPath, f.Path)
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		for _, f := range parsed.Fields {
			cmp := less[f.Path](items[i], items[j])
			if f.Desc {
				cmp = -cmp
			}
			if cmp != 0 {
				return cmp < 0
			}
		}
		return false
	})
	return nil
}

// parseListFilter validates a List request's filter. The empty filter always
// passes; any expression is errFilterUnsupported until the subset is
// documented and wired.
func parseListFilter(filter string) error {
	if filter == "" {
		return nil
	}
	return errFilterUnsupported
}
