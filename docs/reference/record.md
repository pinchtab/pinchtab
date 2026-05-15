# Record

Record browser activity as a video file. Supports GIF (pure Go), WebM, and MP4 (require ffmpeg).

```bash
# Start recording (format from file extension)
curl -X POST http://localhost:9867/record/start \
  -H "Content-Type: application/json" \
  -d '{"format":"gif","fps":5,"quality":80}'

# Check status
curl http://localhost:9867/record/status

# Stop and save
curl -X POST http://localhost:9867/record/stop > recording.gif
```

## Start Response

```json
{
  "status": "recording",
  "format": "gif",
  "fps": 5,
  "quality": 80,
  "tabId": "tab1"
}
```

## Status Response

```json
{
  "active": true,
  "format": "gif",
  "durationSeconds": 12.5,
  "frames": 62,
  "tabId": "tab1",
  "fps": 5
}
```

## API Body Fields (POST /record/start)

- `format`: `gif`, `webm`, or `mp4`. Determined by file extension in CLI.
- `fps`: Frames per second, 1-30 (default 5).
- `quality`: JPEG capture quality 1-100 (default 80).
- `scale`: Resolution multiplier (default 1.0). Values < 1 reduce output size.
- `tabId`: Target a specific tab.

## CLI

- `record start <file>`: Start recording. Format from extension (.gif, .webm, .mp4).
- `record stop`: Stop and save to the path given at start.
- `record status`: Show active recording info.
- `--fps <n>`: Frames per second (default 5).
- `--quality <n>`: JPEG capture quality (default 80).
- `--scale <f>`: Resolution scale (default 1.0).
- `--tab <id>`: Target a specific tab.

## Format Dependencies

| Format | Dependency | Encoding | Notes |
| --- | --- | --- | --- |
| `.gif` | None | Pure Go (Floyd-Steinberg dithering) | Always available; CPU-intensive for long recordings |
| `.webm` | `ffmpeg` | VP8 via ffmpeg pipe | Requires ffmpeg on `$PATH` |
| `.mp4` | `ffmpeg` | H.264 via ffmpeg pipe | Requires ffmpeg on `$PATH` |

GIF encoding runs entirely in-process — no external binary needed. Dithering is CPU-bound; recordings over ~2 minutes at 10 fps can take 30-60 seconds to encode. Frames are capped at 600 (max ~2 minutes at 5 fps) and downscaled if they exceed 1280x720 pixels to limit memory usage.

WebM and MP4 stream frames to ffmpeg during `record stop`. If ffmpeg is not installed, `record start` returns a 400 error with a message indicating the dependency. Install ffmpeg via your package manager (`brew install ffmpeg`, `apt install ffmpeg`, etc.) or use `.gif` as a zero-dependency alternative.

## Notes

- Gated by `security.allowScreencast` (disabled by default). Enable with `pinchtab config set security.allowScreencast true` and restart the server.
- One active recording per bridge instance.

## Related Pages

- [Screenshot](./screenshot.md)
- [PDF](./pdf.md)
