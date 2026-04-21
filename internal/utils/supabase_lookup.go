package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

type cacheRow struct {
	MessageID int64  `json:"message_id"`
	FileID    string `json:"file_id"`
}

var supabaseHTTP = &http.Client{Timeout: 8 * time.Second}

// GetMessageIDForDocID queries the telegram_cache table in Supabase
// for the row whose file_id matches the given Telegram document ID.
// Returns the message_id so the stream bot can fetch it directly.
func GetMessageIDForDocID(ctx context.Context, docID int64) (int, error) {
	supabaseURL := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_SERVICE_KEY")
	if supabaseURL == "" || supabaseKey == "" {
		return 0, fmt.Errorf("SUPABASE_URL or SUPABASE_SERVICE_KEY not set")
	}

	docIDStr := strconv.FormatInt(docID, 10)
	url := fmt.Sprintf(
		"%s/rest/v1/telegram_cache?file_id=eq.%s&select=message_id,file_id&limit=1",
		supabaseURL, docIDStr,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("apikey", supabaseKey)
	req.Header.Set("Authorization", "Bearer "+supabaseKey)
	req.Header.Set("Accept", "application/json")

	resp, err := supabaseHTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("supabase request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("supabase returned %d: %s", resp.StatusCode, body)
	}

	var rows []cacheRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, fmt.Errorf("parse response: %w", err)
	}
	if len(rows) == 0 || rows[0].MessageID == 0 {
		return 0, fmt.Errorf("no message_id found for doc_id=%d (row missing or message_id null)", docID)
	}

	return int(rows[0].MessageID), nil
}
