// Concierge: WhatsApp bot for the homelab streaming group.
//
// Wires WAHA (WhatsApp HTTP API), Jellyseerr, Sonarr, Radarr, and Jellyfin
// together so the group gets welcome messages, slash-style commands via
// @bot mentions, scheduled digests + polls, and richer download notifications.
//
// See docs/DESIGN.md for the architecture and command surface.
package main

import "fmt"

var versionStr = "dev"

func main() {
	// Wiring lands in a follow-up commit. This entry point exists so the
	// module is buildable from day one and CI can compile-check the scaffold.
	fmt.Println("concierge", versionStr, "(scaffold)")
}
