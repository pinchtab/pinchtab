# Docker Deployment

Pinchtab includes a Docker image with bundled Chromium for easy deployment.

## Quick Start

### Using Docker Compose

```bash
# Clone the repository
git clone https://github.com/pinchtab/pinchtab.git
cd pinchtab

# Start with docker-compose
docker-compose up -d

# Check logs
docker-compose logs -f

# Test
curl http://localhost:9867/health
```

### Using Docker CLI

```bash
# Build image
docker build -t pinchtab .

# Run container
docker run -d \
  --name pinchtab \
  -p 9867:9867 \
  -v pinchtab-data:/data \
  --security-opt seccomp=unconfined \
  pinchtab

# With auth token
docker run -d \
  --name pinchtab \
  -p 9867:9867 \
  -v pinchtab-data:/data \
  -e BRIDGE_TOKEN=your-secret-token \
  --security-opt seccomp=unconfined \
  pinchtab
```

## Configuration

Environment variables:
- `BRIDGE_BIND` - Address to bind to (set to `0.0.0.0` in Docker)
- `BRIDGE_PORT` - HTTP port (default: 9867)
- `BRIDGE_TOKEN` - Auth token (optional)
- `BRIDGE_HEADLESS` - Run Chrome headless (default: true in Docker)
- `BRIDGE_STATE_DIR` - State directory (default: /data)
- `BRIDGE_STEALTH` - Stealth level: `light` (default) or `full`
- `BRIDGE_BLOCK_IMAGES` - Block image loading (default: false)
- `BRIDGE_BLOCK_MEDIA` - Block all media (default: false)
- `BRIDGE_NO_ANIMATIONS` - Disable CSS animations (default: false)
- `CHROME_BINARY` - Set automatically in Docker (`/usr/bin/chromium-browser`)
- `CHROME_FLAGS` - Set automatically in Docker (`--no-sandbox --disable-gpu`)

## Architecture

The Docker image:
- Uses Alpine Linux for minimal size
- Includes Chromium browser
- Runs as non-root user
- Uses dumb-init for proper signal handling
- Persists state in `/data` volume

## Profiles & Dashboard Mode

Docker runs Pinchtab in **bridge mode** — a single headless Chrome instance with one default profile. This is the intended setup for containers.

The **profile management** feature (dashboard mode, multiple named Chrome profiles) is designed for **desktop/workstation** use where you might manage multiple browser identities from a GUI. It doesn't apply to Docker deployments:

- Containers are ephemeral — profiles don't persist unless you mount `/data`
- There's no user desktop or personal Chrome to coexist with
- One container = one browser instance, which is the right model for automation

If you need multiple isolated browser instances, run multiple containers rather than using the dashboard's profile system.

## Security Notes

The container requires `--security-opt seccomp=unconfined` for Chrome to function properly. This is a limitation of running Chrome in containers.

For production use:
1. Always set `BRIDGE_TOKEN`
2. Use a reverse proxy with TLS
3. Limit container resources
4. Run on isolated networks

## Troubleshooting

### Chrome crashes
Increase memory limit:
```yaml
mem_limit: 4g
```

### Font issues
The image includes basic fonts. For specific fonts:
```dockerfile
RUN apk add --no-cache ttf-liberation ttf-dejavu
```

### Performance
For better performance:
```yaml
shm_size: '2gb'  # Shared memory for Chrome
```