package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// pageCursor encodes the position of the last returned item.
type pageCursor struct {
	SortVal string `json:"s"`
	ID      string `json:"i"`
}

func encodeCursor(sortVal, id string) string {
	c := pageCursor{SortVal: sortVal, ID: id}
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (pageCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, err
	}
	var c pageCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return pageCursor{}, err
	}
	if c.ID == "" {
		return pageCursor{}, fmt.Errorf("cursor missing id")
	}
	return c, nil
}

// paginationParams holds parsed pagination query parameters.
type paginationParams struct {
	CursorAt string
	CursorID string
	Limit    int
}

func parsePaginationParams(r *http.Request) (paginationParams, error) {
	p := paginationParams{Limit: 25}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 100 {
			return p, fmt.Errorf("limit must be between 1 and 100")
		}
		p.Limit = limit
	}

	if cursorStr := r.URL.Query().Get("cursor"); cursorStr != "" {
		if len(cursorStr) > 4096 {
			return p, fmt.Errorf("cursor too long")
		}
		c, err := decodeCursor(cursorStr)
		if err != nil {
			return p, fmt.Errorf("invalid cursor")
		}
		p.CursorAt = c.SortVal
		p.CursorID = c.ID
	}

	return p, nil
}

// paginationMeta is the pagination section of a list response.
type paginationMeta struct {
	HasMore    bool    `json:"hasMore"`
	NextCursor *string `json:"nextCursor"`
}

// writeJSON encodes v as JSON to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// decodeJSONStrict decodes JSON from the request body, rejecting unknown fields.
func decodeJSONStrict(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
