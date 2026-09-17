package dto

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
