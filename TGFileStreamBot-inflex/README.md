# TGFileStreamBot — InFlex Edition

All original functionality is preserved. One new route has been added to
support the InFlex serverless architecture (GitHub Actions → Telethon upload).

## New Route

```
GET /stream/doc/:docID
GET /stream/doc/:docID?d=true    (force download)
```

**`:docID`** is the Telethon `message.document.id` stored by `worker.py` as
`file_id` in the Supabase `telegram_cache` table.

The route scans `LOG_CHANNEL` for the matching message, then streams it with
the same high-performance `StreamPipe` used by the original `/stream/:messageID`
route. Results are cached in-memory for 1 hour so video seeks are free.

No hash parameter is needed — the channel is private, so having the document
ID from Supabase is sufficient authorisation.

## Flutter usage

```dart
final streamUrl = '$streamBotHost/stream/doc/$fileId';
// fileId = the value from telegram_cache.file_id after status == 'cached'
```

## Environment Variables (unchanged)

| Variable | Required | Description |
|---|---|---|
| `API_ID` | ✅ | Telegram API ID from my.telegram.org |
| `API_HASH` | ✅ | Telegram API Hash |
| `BOT_TOKEN` | ✅ | Bot token (must be admin in LOG_CHANNEL) |
| `LOG_CHANNEL` | ✅ | Channel ID where worker.py uploads files |
| `HOST` | ✅ on Render | Your Render URL e.g. `https://xxx.onrender.com` |
| `PORT` | optional | Default 8080 |
| `HASH_LENGTH` | optional | Default 6 |
| `USE_SESSION_FILE` | optional | Default true |
| `USER_SESSION` | optional | Pyrogram-format session for userbot features |

## Original routes (unchanged)

```
GET /stream/:messageID?hash=<hash>   Original route — still works
```

## Render Deployment

1. Push this repo to GitHub
2. Create a new Render **Web Service** → connect the repo
3. **Build Command:** `go build -o fsb ./cmd/fsb`
4. **Start Command:** `./fsb run`
5. Set the environment variables above in the Render dashboard
6. Set `HOST` to your Render service URL
