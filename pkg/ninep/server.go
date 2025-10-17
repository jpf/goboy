// ABOUTME: 9P server implementation for exposing emulator state over network.
// ABOUTME: Serves virtual filesystem using native p9.File with fskit for state tree.

package ninep

import (
	"fmt"
	"net"

	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

const readmeContent = `GoBoy 9P Interface
===================

This filesystem exposes the Game Boy emulator's internal state for
inspection and manipulation using standard Unix tools.

STRUCTURE
---------
/README                  This file
/ctl                     Control interface (read status, write commands)
/state/                  Emulator state directory
  memory/
    vram                 Video RAM (16KB binary)

CONTROL COMMANDS
----------------
Read 'ctl' to see current status:
  cat ctl

Write commands to 'ctl':
  echo pause > ctl   - Pause emulation
  echo resume > ctl  - Resume emulation

STATE FILES
-----------
state/memory/vram        Video RAM (16KB binary, both banks concatenated)
                         Bytes 0x0000-0x1FFF: Bank 0
                         Bytes 0x2000-0x3FFF: Bank 1 (CGB only)

EXAMPLES
--------
# Read VRAM (live, may have brief race conditions during reads)
xxd /mnt/goboy/state/memory/vram | head

# Surgical edit at specific offset (pause for consistency)
echo pause > /mnt/goboy/ctl
echo -n '\xFF\xFF' | dd of=/mnt/goboy/state/memory/vram bs=1 seek=1024 conv=notrunc
echo resume > /mnt/goboy/ctl

# Full VRAM corruption (visual test - will show graphical glitches)
dd if=/dev/random of=/mnt/goboy/state/memory/vram bs=16384 count=1

NOTES
-----
- Reads are instantaneous and reflect live state
- VRAM reads may see brief inconsistencies (pause before reading for guaranteed consistency)
- Writes are queued and applied at next frame boundary (~16ms latency)
- No validation - invalid data will corrupt or crash the emulator
- Multiple emulator instances can run on different ports

See 9p-spec.md for the complete interface specification.
`

// Start launches the 9P server on the specified port.
// Returns error if server fails to start.
func Start(gameboy *gb.Gameboy, port int) error {
	// Create TCP listener
	addr := fmt.Sprintf(":%d", port)
	serverSocket, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	// Create p9 attacher with native p9.File implementation
	attacher := NewP9Attacher(gameboy)
	server := p9.NewServer(attacher)

	// Launch in goroutine
	go func() {
		if err := server.Serve(serverSocket); err != nil {
			fmt.Printf("9P server error: %v\n", err)
		}
	}()

	return nil
}
