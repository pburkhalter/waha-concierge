# concierge

WhatsApp bot for the homelab streaming group. Wires WAHA, Jellyseerr,
Sonarr, Radarr, and Jellyfin together so the group gets:

- **Welcome message** on join
- **`@bot` commands** for search, request, status, recently-added, etc.
- **Scheduled posts** — weekly digest, weekly poll
- **Enhanced notifications** — episode grouping, requester @mention, poster image

Designed to run alongside [arrarr](https://github.com/pburkhalter/Arrarr) and
share its operational style (single Go binary, distroless image, TrueNAS
custom app deploy).

## Status

🚧 **Design phase.** See [docs/DESIGN.md](docs/DESIGN.md) for the architecture
and command surface. Scaffold lives in `cmd/` + `internal/`; implementation
lands incrementally.

## Configuration

Env vars are documented in [docs/DESIGN.md § Config](docs/DESIGN.md#config-env).

## License

MIT.
