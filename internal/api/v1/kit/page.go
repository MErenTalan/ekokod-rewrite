package kit

import (
	"encoding/base64"
	"encoding/json"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const (
	defaultLimit = 50
	maxLimit     = 500
)

// PageRequest is the pagination every list route embeds (R153).
type PageRequest struct {
	Limit  *int32 `query:"limit" json:"-" minimum:"1" maximum:"500"`
	Cursor string `query:"cursor" json:"-"`
}

// Page is a list response.
type Page[T any] struct {
	Items      []T     `json:"items" required:"true"`
	NextCursor *string `json:"next_cursor"`
}

type cursor struct {
	Offset int32 `json:"o"`
}

// Resolve returns the store page to request (limit+1 rows) and the page size;
// max caps the limit below the repository's own cap.
func (p PageRequest) Resolve(maxRows int32) (store.Page, int32, error) {
	limit := int32(defaultLimit)
	ceiling := min(int32(maxLimit), maxRows)
	if p.Limit != nil {
		if *p.Limit < 1 || *p.Limit > ceiling {
			return store.Page{}, 0, ErrInvalidLimit
		}
		limit = *p.Limit
	}
	limit = min(limit, ceiling)
	var offset int32
	if p.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(p.Cursor)
		var c cursor
		if err != nil || json.Unmarshal(raw, &c) != nil || c.Offset < 0 {
			return store.Page{}, 0, ErrInvalidCursor
		}
		offset = c.Offset
	}
	return store.Page{Limit: limit + 1, Offset: offset}, limit, nil
}

// PageOf trims the extra row Resolve asked for and sets next_cursor when it came back.
func PageOf[T any](items []T, page store.Page, limit int32) Page[T] {
	out := Page[T]{Items: items}
	if out.Items == nil {
		out.Items = []T{}
	}
	if int32(len(out.Items)) > limit {
		out.Items = out.Items[:limit]
		raw, _ := json.Marshal(cursor{Offset: page.Offset + limit})
		next := base64.RawURLEncoding.EncodeToString(raw)
		out.NextCursor = &next
	}
	return out
}
