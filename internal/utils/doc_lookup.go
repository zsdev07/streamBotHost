package utils

// ─────────────────────────────────────────────────────────────────────────────
// FileFromDocID — resolve a Telegram File by its document ID
// ─────────────────────────────────────────────────────────────────────────────
//
// worker.py (Telethon) stores `message.document.id` as `file_id` in Supabase.
// This function scans LOG_CHANNEL messages to find the one whose document ID
// matches, then returns the same types.File struct the rest of the bot uses.
//
// Results are cached in the existing freecache for 1 hour so video seeks
// (which call this repeatedly) hit memory, not Telegram.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"EverythingSuckz/fsb/internal/cache"
	"EverythingSuckz/fsb/internal/types"
	"context"
	"fmt"

	"github.com/celestix/gotgproto"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// FileFromDocID finds the message in LOG_CHANNEL whose document.ID == docID
// and returns its File struct for streaming.
func FileFromDocID(ctx context.Context, client *gotgproto.Client, docID int64) (*types.File, error) {
	log := Logger.Named("FileFromDocID")

	// Cache key uses docID only — independent of which worker bot handles it
	cacheKey := fmt.Sprintf("doc:%d", docID)

	var cached types.File
	if err := cache.GetCache().Get(cacheKey, &cached); err == nil {
		log.Debug("Cache hit", zap.Int64("docID", docID))
		return &cached, nil
	}

	log.Debug("Cache miss — scanning channel", zap.Int64("docID", docID))

	// Resolve the channel peer
	channel, err := GetLogChannelPeer(ctx, client.API(), client.PeerStorage)
	if err != nil {
		return nil, fmt.Errorf("channel peer: %w", err)
	}

	// Search in batches of 100 messages, newest-first, up to 10 batches (1000 msgs)
	const batchSize = 100
	const maxBatches = 10
	var offsetID int = 0

	for batch := 0; batch < maxBatches; batch++ {
		res, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:      &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
			OffsetID:  offsetID,
			Limit:     batchSize,
			AddOffset: 0,
			MaxID:     0,
			MinID:     0,
		})
		if err != nil {
			return nil, fmt.Errorf("GetHistory batch %d: %w", batch, err)
		}

		msgs, ok := res.(*tg.MessagesChannelMessages)
		if !ok {
			return nil, fmt.Errorf("unexpected GetHistory response type")
		}
		if len(msgs.Messages) == 0 {
			break // no more messages
		}

		for _, rawMsg := range msgs.Messages {
			msg, ok := rawMsg.(*tg.Message)
			if !ok {
				continue
			}
			doc, ok := msg.Media.(*tg.MessageMediaDocument)
			if !ok {
				continue
			}
			document, ok := doc.Document.AsNotEmpty()
			if !ok {
				continue
			}
			if document.ID != docID {
				continue
			}

			// Found it — build File struct
			file, err := FileFromMedia(msg.Media)
			if err != nil {
				return nil, fmt.Errorf("FileFromMedia: %w", err)
			}

			// Cache for 1 hour
			_ = cache.GetCache().Set(cacheKey, file, 3600)

			// Also cache by messageID so /stream/:messageID still works
			msgCacheKey := fmt.Sprintf("file:%d:%d", msg.ID, client.Self.ID)
			_ = cache.GetCache().Set(msgCacheKey, file, 3600)

			log.Info("Resolved docID to message",
				zap.Int64("docID", docID),
				zap.Int("messageID", msg.ID),
				zap.String("fileName", file.FileName),
			)
			return file, nil
		}

		// Move offset to the ID of the last message in this batch
		lastMsg := msgs.Messages[len(msgs.Messages)-1]
		if m, ok := lastMsg.(*tg.Message); ok {
			offsetID = m.ID
		} else {
			break
		}
	}

	return nil, fmt.Errorf("document %d not found in last %d messages", docID, maxBatches*batchSize)
}
