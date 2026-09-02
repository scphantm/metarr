package services

import (
	"errors"
	"testing"

	"connectrpc.com/connect"
)

// The List helpers back every collection service's AIP-158 / 132 / 160 List
// contract, so these pin the window arithmetic, the round-trippable
// page_token, the order_by field-name gate, and the deferred-filter code.

func TestPaginateSlice_WalksEveryPageThenStops(t *testing.T) {
	items := []int{0, 1, 2, 3, 4, 5, 6}

	var seen []int
	token := ""
	pages := 0
	for {
		page, next, err := paginateSlice(items, 3, token)
		if err != nil {
			t.Fatalf("paginateSlice: %v", err)
		}
		seen = append(seen, page...)
		pages++
		if next == "" {
			break
		}
		token = next
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
	}

	if pages != 3 {
		t.Errorf("walked %d pages, want 3 (3+3+1)", pages)
	}
	if len(seen) != len(items) {
		t.Fatalf("saw %v, want every item once", seen)
	}
	for i, v := range seen {
		if v != items[i] {
			t.Fatalf("item %d out of order: saw %v", i, seen)
		}
	}
}

func TestPaginateSlice_UnsetPageSizeUsesTheDefault(t *testing.T) {
	items := make([]int, defaultListPageSize+5)
	page, next, err := paginateSlice(items, 0, "")
	if err != nil {
		t.Fatalf("paginateSlice: %v", err)
	}
	if len(page) != defaultListPageSize {
		t.Errorf("page size = %d, want the default %d", len(page), defaultListPageSize)
	}
	if next == "" {
		t.Error("expected a next page token with items still to come")
	}
}

func TestPaginateSlice_RejectsAGarbagePageToken(t *testing.T) {
	_, _, err := paginateSlice([]int{1, 2, 3}, 2, "not-a-real-token")
	if !errors.Is(err, errBadPageToken) {
		t.Fatalf("got %v, want errBadPageToken", err)
	}
	if connect.CodeOf(aipConnectError(err)) != connect.CodeInvalidArgument {
		t.Errorf("errBadPageToken maps to %v, want InvalidArgument", connect.CodeOf(aipConnectError(err)))
	}
}

func TestOrderBySlice_SortsByNamedFieldsAndHonoursDesc(t *testing.T) {
	type row struct {
		name string
		age  int
	}
	rows := []row{{"c", 30}, {"a", 30}, {"b", 20}}
	less := map[string]func(a, b row) int{
		"name": func(a, b row) int { return cmpStr(a.name, b.name) },
		"age":  func(a, b row) int { return a.age - b.age },
	}

	if err := orderBySlice(rows, "age desc, name", less); err != nil {
		t.Fatalf("orderBySlice: %v", err)
	}
	got := []string{rows[0].name, rows[1].name, rows[2].name}
	want := []string{"a", "c", "b"} // age 30 before 20; within 30, name asc
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestOrderBySlice_RejectsAnUnknownField(t *testing.T) {
	rows := []int{1, 2}
	err := orderBySlice(rows, "weight", map[string]func(a, b int) int{
		"value": func(a, b int) int { return a - b },
	})
	if !errors.Is(err, errUnknownPath) {
		t.Fatalf("got %v, want errUnknownPath", err)
	}
}

func TestOrderBySlice_EmptyOrderByIsANoOp(t *testing.T) {
	rows := []int{3, 1, 2}
	if err := orderBySlice(rows, "", nil); err != nil {
		t.Fatalf("orderBySlice: %v", err)
	}
	if rows[0] != 3 || rows[1] != 1 || rows[2] != 2 {
		t.Errorf("an empty order_by reordered the slice: %v", rows)
	}
}

func TestParseListFilter_EmptyPassesAnythingElseIsUnimplemented(t *testing.T) {
	if err := parseListFilter(""); err != nil {
		t.Fatalf("empty filter rejected: %v", err)
	}
	err := parseListFilter(`display_name = "nas-01"`)
	if !errors.Is(err, errFilterUnsupported) {
		t.Fatalf("got %v, want errFilterUnsupported", err)
	}
	if got := connect.CodeOf(aipConnectError(err)); got != connect.CodeUnimplemented {
		t.Errorf("filter error maps to %v, want Unimplemented", got)
	}
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
