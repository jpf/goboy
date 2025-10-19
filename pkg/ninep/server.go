// ABOUTME: 9P server implementation for exposing emulator state over network.
// ABOUTME: Serves virtual filesystem using native p9.File with fskit for state tree.

package ninep

import (
	"fmt"
	"net"
	"os"

	"github.com/hugelgupf/p9/p9"
	"github.com/Humpheh/goboy/pkg/gb"
)

// verbose controls whether to log 9P operations to stderr
var verbose bool

// logf prints formatted message to stderr if verbose logging is enabled
func logf(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, "[9P] "+format+"\n", args...)
	}
}

const readmeContent = `GoBoy 9P Interface
==================

This filesystem exposes the Game Boy emulator's internal state for inspection
and manipulation using standard Unix tools.

STRUCTURE
---------
/README                  This file
/ctl                     Control interface (read status, write commands)
/state/                  Emulator state directory
  cartridge/
    info                 ROM header metadata (text format, read-only)
    ram                  Cartridge RAM/save data (binary, size varies by MBC)
  cpu                    CPU registers and program counter (text format)
  memory/
    vram                 Video RAM (16KB binary, both banks)
    wram                 Work RAM (36KB binary, 8 banks + gap)
    oam                  Object Attribute Memory (256B binary)
    highram              High RAM + hardware registers (256B binary)
    state                Memory banking state (text format)

CONTROL COMMANDS
----------------
Read 'ctl' to see current status and available commands.
Write commands to 'ctl':
  echo pause > ctl   - Pause emulation
  echo resume > ctl  - Resume emulation
  echo reset > ctl   - Reset to power-on state

STATE FILES
-----------
Text files use KEY=0xVALUE format with hex values.
Binary files are raw byte dumps matching internal memory layout.

state/cartridge/info     ROM header metadata (text format, read-only)
                         title - Game title from ROM (ASCII string)
                         type - MBC type (ROM, MBC1, MBC2, MBC3, MBC5, UNKNOWN)
                         romSize - ROM size in bytes (decimal)
                         ramSize - RAM size in bytes (decimal)
                         cgbSupport - CGB flag (8-bit hex)
                         sgbSupport - SGB flag (8-bit hex)
                         hasBattery - Battery backup flag (8-bit hex)
                         headerChecksum - ROM header checksum (8-bit hex)
                         globalChecksum - Global ROM checksum (16-bit hex)

                         Example:
                         # ROM Information
                         title=POKEMON BLUE
                         type=MBC3
                         romSize=1048576
                         ramSize=32768

                         # Features
                         cgbSupport=0x00
                         sgbSupport=0x00
                         hasBattery=0x01

                         # Checksums
                         headerChecksum=0x3C
                         globalChecksum=0xB0A2

state/cpu                CPU state (text format, key=value pairs)
                         AF, BC, DE, HL - Register pairs (16-bit hex)
                         SP - Stack pointer (16-bit hex)
                         PC - Program counter (16-bit hex)
                         Divider - Timer divider register (16-bit hex)

                         Example:
                         AF=0x01B0
                         BC=0x0013
                         PC=0x0100
                         Divider=0x00AB

state/memory/vram        Video RAM (16KB binary, both banks concatenated)
                         Bytes 0x0000-0x1FFF: Bank 0
                         Bytes 0x2000-0x3FFF: Bank 1 (CGB only)

state/memory/wram        Work RAM (36KB binary, 8 banks)
                         Bytes 0x0000-0x0FFF: Bank 0 (fixed)
                         Bytes 0x1000-0x1FFF: Unused gap (implementation detail)
                         Bytes 0x2000-0x2FFF: Bank 1 (switchable)
                         Bytes 0x3000-0x3FFF: Bank 2
                         Bytes 0x4000-0x4FFF: Bank 3
                         Bytes 0x5000-0x5FFF: Bank 4
                         Bytes 0x6000-0x6FFF: Bank 5
                         Bytes 0x7000-0x7FFF: Bank 6
                         Bytes 0x8000-0x8FFF: Bank 7

state/memory/oam         Object Attribute Memory (256 bytes binary)
                         Sprite data at 0xFE00-0xFE9F (160 bytes actively used)

state/memory/highram     High RAM + hardware registers (256 bytes binary)
                         Maps to GB address 0xFF00-0xFFFF
                         Includes timer, sound, PPU, and other I/O registers
                         WARNING: Registers change every CPU cycle

state/memory/state       Memory banking state (text format, key=value pairs)
                         VRAMBank (0-1)
                         WRAMBank (0-7)
                         hdmaLength (0-255)
                         hdmaActive (0x00=false, 0x01=true)

state/cartridge/ram      Cartridge RAM/save data (binary format, offset-based access)
                         Size varies by cartridge type:
                         - ROM-only: No RAM (file not present)
                         - MBC2: 8KB (0x2000 bytes)
                         - MBC1: 32KB (0x8000 bytes)
                         - MBC3: 32KB (0x8000 bytes)
                         - MBC5: 128KB (0x20000 bytes)

                         Example:
                         # Backup save data
                         cat state/cartridge/ram > ~/backup.sav

                         # Restore save data
                         cat ~/backup.sav > state/cartridge/ram

                         # Hex dump save data
                         xxd state/cartridge/ram | head

EXAMPLES
--------
# Read ROM metadata
cat /mnt/goboy/state/cartridge/info

# Read CPU state
cat /mnt/goboy/state/cpu

# Modify program counter
echo "PC=0x0150" > /mnt/goboy/state/cpu

# Set multiple registers at once
echo -e "AF=0x01B0\nBC=0x0013\nPC=0x0100" > /mnt/goboy/state/cpu

# Save complete emulator state (memory + CPU)
echo pause > /mnt/goboy/ctl
tar -czf fullstate.tar.gz -C /mnt/goboy/state cpu memory/
echo resume > /mnt/goboy/ctl

# Restore complete emulator state
echo pause > /mnt/goboy/ctl
tar -xzf fullstate.tar.gz -C /mnt/goboy/state
echo resume > /mnt/goboy/ctl

# Read memory banking state
cat /mnt/goboy/state/memory/state

# Change WRAM bank
echo "WRAMBank=0x03" > /mnt/goboy/state/memory/state

# Modify sprite data
echo pause > /mnt/goboy/ctl
echo -n '\x10\x20\x30\x40' | dd of=/mnt/goboy/state/memory/oam bs=1 seek=0 conv=notrunc
echo resume > /mnt/goboy/ctl

##Dump all memory regions
xxd /mnt/goboy/state/memory/wram > wram.hex
xxd /mnt/goboy/state/memory/oam > oam.hex
xxd /mnt/goboy/state/memory/highram > highram.hex

# Backup cartridge save data
cat /mnt/goboy/state/cartridge/ram > ~/pokemon-blue-save.sav

# Restore cartridge save data
cat ~/pokemon-blue-save.sav > /mnt/goboy/state/cartridge/ram

NOTES
-----
- Reads are instantaneous and reflect live state
- Brief race conditions possible during reads (pause for consistency)
- HighRAM contains hardware registers that change every CPU cycle
- Writes are validated THEN queued (applied at next frame boundary ~16ms)
- Invalid writes return errors immediately with clear messages
- Pause emulator for atomic save/restore operations
- Multiple emulator instances can run on different ports
`

// Start launches the 9P server on the specified port.
// Returns error if server fails to start.
func Start(gameboy *gb.Gameboy, port int, enableVerbose bool) error {
	// Set global verbose flag
	verbose = enableVerbose

	// Create TCP listener
	addr := fmt.Sprintf(":%d", port)
	serverSocket, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}

	logf("9P server listening on %s", addr)

	// Create p9 attacher with native p9.File implementation
	attacher := NewP9Attacher(gameboy)
	server := p9.NewServer(attacher)

	// Launch in goroutine with connection logging
	go func() {
		for {
			conn, err := serverSocket.Accept()
			if err != nil {
				logf("Accept error: %v", err)
				continue
			}

			logf("New connection from %s", conn.RemoteAddr())

			// Handle connection in goroutine
			go func(c net.Conn) {
				defer func() {
					logf("Connection closed from %s", c.RemoteAddr())
					c.Close()
				}()

				if err := server.Handle(c, c); err != nil {
					logf("Connection error from %s: %v", c.RemoteAddr(), err)
				}
			}(conn)
		}
	}()

	return nil
}
