# Development Setup

This document has been consolidated into the main contributor guide:

- [docs/guides/contributing.md](docs/guides/contributing.md)

Use that page as the source of truth for environment setup, building, testing, hooks, and contributor workflow.

If the dashboard you are testing looks stale, see the "Dashboard Bundle Stamp" section
there: `./dev build dashboard` rebuilds and stamps the embedded bundle, `GET /health`
reports the stamp as `dashboardBuild`, and the unit suite fails when it no longer
matches `dashboard/`.
