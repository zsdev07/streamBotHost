package utils

import (
	"EverythingSuckz/fsb/internal/cache"
	"EverythingSuckz/fsb/internal/types"
	"context"
	"fmt"

	"github.com/celestix/gotgproto"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// FileFromDocID resolves a Telegram File by its document ID.
// It queries Supabase for the message_id paired to this doc ID,
// then fetches that single message directly — bots can do this,
// unlike MessagesGetHistory which is user-only.
func FileFromDocID(ctx context.Context, client *gotgproto.Client, docID int64) (*types.File, error) {
	log := Logger.Named("FileFromDocID")

	cacheKey := fmt.Sprintf("doc:%d", docID)

	var cached types.File
	if err := cache.GetCache().Get(cacheKey, &cached); err == nil {
		log.Debug("Cache hit", zap.Int64("docID", docID))
		return &cached, nil
	}

	log.Debug("Cache miss — looking up message_id from Supabase", zap.Int64("docID", docID))

	// Look up the message_id stored by worker.py
	msgID, err := GetMessageIDForDocID(ctx, docID)
	if err != nil {
		return nil, fmt.Errorf("supabase lookup: %w", err)
	}

	log.Debug("Fetching message directly", zap.Int64("docID", docID), zap.Int("msgID", msgID))

	file, err := FileFromMessage(ctx, client, msgID)
	if err != nil {
		return nil, fmt.Errorf("FileFromMessage(%d): %w", msgID, err)
	}

	// Cache by doc ID too
	_ = cache.GetCache().Set(cacheKey, file, 3600)

	log.Info("Resolved docID via Supabase message_id",
		zap.Int64("docID", docID),
		zap.Int("msgID", msgID),
		zap.String("fileName", file.FileName),
	)
	return file, nil
}
