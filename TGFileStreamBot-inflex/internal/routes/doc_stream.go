package routes

// ─────────────────────────────────────────────────────────────────────────────
// /stream/doc/:docID  — InFlex direct-stream route
// ─────────────────────────────────────────────────────────────────────────────
//
// The GitHub Actions worker (worker.py) uploads a file via Telethon and
// stores doc.id (the Telegram document ID) as `file_id` in Supabase.
// The Flutter app passes that ID here — no hash required.
//
// Route:  GET /stream/doc/<file_id>
//         GET /stream/doc/<file_id>?d=true   (force-download)
// ─────────────────────────────────────────────────────────────────────────────

import (
	"EverythingSuckz/fsb/internal/bot"
	"EverythingSuckz/fsb/internal/stream"
	"EverythingSuckz/fsb/internal/types"
	"EverythingSuckz/fsb/internal/utils"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gotd/td/tg"
	range_parser "github.com/quantumsheep/range-parser"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
)

// LoadDocStream registers /stream/doc/:docID.
// Must start with "Load" — routes.Load() discovers handlers via reflection.
func (e *allRoutes) LoadDocStream(r *Route) {
	docLog := e.log.Named("DocStream")
	defer docLog.Info("Loaded /stream/doc/:docID route")
	r.Engine.GET("/stream/doc/:docID", makeDocStreamHandler(docLog))
}

func makeDocStreamHandler(docLog *zap.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		w := ctx.Writer
		r := ctx.Request

		// 1. Parse docID
		docID, err := strconv.ParseInt(ctx.Param("docID"), 10, 64)
		if err != nil {
			http.Error(w, "invalid docID: "+err.Error(), http.StatusBadRequest)
			return
		}

		// 2. Resolve File from document ID (searches LOG_CHANNEL, cached 1h)
		worker := bot.GetNextWorker()
		file, err := utils.TimeFuncWithResult(docLog, "FileFromDocID", func() (*types.File, error) {
			return utils.FileFromDocID(ctx, worker.Client, docID)
		})
		if err != nil {
			docLog.Error("FileFromDocID failed", zap.Int64("docID", docID), zap.Error(err))
			http.Error(w, "file not found: "+err.Error(), http.StatusNotFound)
			return
		}

		// 3. Photo path (FileSize == 0) — send whole file at once
		if file.FileSize == 0 {
			res, err2 := worker.Client.API().UploadGetFile(ctx, &tg.UploadGetFileRequest{
				Location: file.Location,
				Offset:   0,
				Limit:    1024 * 1024,
			})
			if err2 != nil {
				http.Error(w, err2.Error(), http.StatusInternalServerError)
				return
			}
			result, ok := res.(*tg.UploadFile)
			if !ok {
				http.Error(w, "unexpected upload response", http.StatusInternalServerError)
				return
			}
			ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file.FileName))
			if r.Method != "HEAD" {
				ctx.Data(http.StatusOK, file.MimeType, result.GetBytes())
			}
			return
		}

		// 4. Range handling
		ctx.Header("Accept-Ranges", "bytes")
		var start, end int64
		if rangeHeader := r.Header.Get("Range"); rangeHeader == "" {
			start = 0
			end = file.FileSize - 1
			w.WriteHeader(http.StatusOK)
		} else {
			ranges, err2 := range_parser.Parse(file.FileSize, rangeHeader)
			if err2 != nil {
				http.Error(w, err2.Error(), http.StatusBadRequest)
				return
			}
			start = ranges[0].Start
			end = ranges[0].End
			ctx.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, file.FileSize))
			w.WriteHeader(http.StatusPartialContent)
		}

		contentLength := end - start + 1
		mimeType := file.MimeType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		ctx.Header("Content-Type", mimeType)
		ctx.Header("Content-Length", strconv.FormatInt(contentLength, 10))

		disposition := "inline"
		if ctx.Query("d") == "true" {
			disposition = "attachment"
		}
		ctx.Header("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", disposition, file.FileName))

		// 5. Stream
		if r.Method != "HEAD" {
			pipe, err2 := stream.NewStreamPipe(ctx, worker.Client, file.Location, start, end, docLog)
			if err2 != nil {
				docLog.Error("Failed to create stream pipe", zap.Error(err2))
				return
			}
			defer pipe.Close()
			if _, err2 = io.CopyN(w, pipe, contentLength); err2 != nil {
				if !utils.IsClientDisconnectError(err2) {
					docLog.Error("Stream copy error", zap.Error(err2))
				}
			}
		}
	}
}
