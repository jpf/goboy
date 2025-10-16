// ABOUTME: 9P server implementation for exposing emulator state over network.
// ABOUTME: Serves virtual filesystem using native p9.File implementation.

package ninep

import (
	"fmt"
	"net"

	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

const readmeContent = `GoBoy 9P Interface (Proof-of-Concept)
========================================

This filesystem exposes the Game Boy emulator's control interface for
inspection and manipulation using standard Unix tools.

STRUCTURE
---------
/README              This file
/ctl                 Control interface (read status, write commands)

CONTROL COMMANDS
----------------
Read 'ctl' to see current status:
  cat ctl

Write commands to 'ctl':
  echo pause > ctl   - Pause emulation
  echo resume > ctl  - Resume emulation

STATUS
------
This is a proof-of-concept implementation. Future versions will include:
- Complete emulator state under /state/ directory
- Save state functionality (tar -czf savestate.tar.gz state/)
- Reset command support
- ROM loading capability

NOTES
-----
- Reads are instantaneous and reflect live state
- Writes are applied at next frame boundary (~16ms latency)
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

	// Create p9 attacher
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
