package api

import (
	"testing"
	"time"
)

// Clients poll /api/wiki/pages/{id}/content every 15 s per open page. Before the
// ETag, one reader on a 24 KB page pulled ~100 KB/min of unchanged markdown.
func TestWikiContentETag(t *testing.T) {
	at := time.Date(2026, 9, 17, 16, 34, 46, 123, time.UTC)

	t.Run("stable for the same page and timestamp", func(t *testing.T) {
		if wikiContentETag(175, at) != wikiContentETag(175, at) {
			t.Fatal("etag must be stable, or every poll re-downloads")
		}
	})

	t.Run("changes when the page is edited", func(t *testing.T) {
		if wikiContentETag(175, at) == wikiContentETag(175, at.Add(time.Nanosecond)) {
			t.Fatal("etag must move with updated_at, or an edit is never picked up")
		}
	})

	t.Run("differs between pages", func(t *testing.T) {
		if wikiContentETag(175, at) == wikiContentETag(167, at) {
			t.Fatal("two pages saved in the same instant must not share a validator")
		}
	})

	t.Run("is a quoted entity tag", func(t *testing.T) {
		got := wikiContentETag(175, at)
		if len(got) < 2 || got[0] != '"' || got[len(got)-1] != '"' {
			t.Fatalf("ETag must be quoted per RFC 9110, got %s", got)
		}
	})
}
