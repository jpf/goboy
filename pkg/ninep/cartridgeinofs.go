// ABOUTME: Read-only filesystem for cartridge metadata (ROM header info).
// ABOUTME: Parses ROM header bytes and exposes title, type, sizes, checksums.

package ninep

import (
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/Humpheh/goboy/pkg/gb"
)

type cartridgeInfoFS struct {
	gb *gb.Gameboy
}

func newCartridgeInfoFS(gameboy *gb.Gameboy) *cartridgeInfoFS {
	return &cartridgeInfoFS{gb: gameboy}
}

func (fsys *cartridgeInfoFS) Open(name string) (fs.File, error) {
	if name != "." && name != "" {
		return nil, fs.ErrNotExist
	}
	return &cartridgeInfoFile{parent: fsys}, nil
}

type cartridgeInfoFile struct {
	parent  *cartridgeInfoFS
	readPos int
}

func (f *cartridgeInfoFile) Read(buf []byte) (int, error) {
	f.parent.gb.Mu.RLock()
	cart := f.parent.gb.GetCartridge()
	if cart == nil {
		f.parent.gb.Mu.RUnlock()
		return 0, fmt.Errorf("no cartridge loaded")
	}

	// Parse ROM header
	title := cart.GetName()
	cartType := cart.Read(0x0147)
	romSizeByte := cart.Read(0x0148)
	ramSizeByte := cart.Read(0x0149)
	cgbFlag := cart.Read(0x0143)
	sgbFlag := cart.Read(0x0146)
	headerChecksum := cart.Read(0x014D)
	globalChecksum := uint16(cart.Read(0x014E))<<8 | uint16(cart.Read(0x014F))

	f.parent.gb.Mu.RUnlock()

	// Decode ROM size (0x00 = 32KB, 0x01 = 64KB, 0x02 = 128KB, etc.)
	romSize := 32 * 1024 * (1 << romSizeByte)

	// Decode RAM size
	var ramSize int
	switch ramSizeByte {
	case 0x00:
		ramSize = 0
	case 0x01:
		ramSize = 2 * 1024 // Unused in real carts
	case 0x02:
		ramSize = 8 * 1024
	case 0x03:
		ramSize = 32 * 1024
	case 0x04:
		ramSize = 128 * 1024
	case 0x05:
		ramSize = 64 * 1024
	default:
		ramSize = 0
	}

	// Decode cartridge type
	mbcType, hasBattery := decodeCartridgeType(cartType)

	// Format as text
	content := fmt.Sprintf(`# ROM Information
title=%s
type=%s
romSize=%d
ramSize=%d

# Features
cgbSupport=0x%02X
sgbSupport=0x%02X
hasBattery=0x%02X

# Checksums
headerChecksum=0x%02X
globalChecksum=0x%04X
`, title, mbcType, romSize, ramSize, cgbFlag, sgbFlag, boolToHex(hasBattery), headerChecksum, globalChecksum)

	// Handle offset reads
	if f.readPos >= len(content) {
		return 0, io.EOF
	}

	n := copy(buf, content[f.readPos:])
	f.readPos += n

	if f.readPos >= len(content) {
		return n, io.EOF
	}
	return n, nil
}

func (f *cartridgeInfoFile) Write(data []byte) (int, error) {
	return 0, fs.ErrPermission // Read-only
}

func (f *cartridgeInfoFile) Close() error {
	return nil
}

func (f *cartridgeInfoFile) Stat() (fs.FileInfo, error) {
	return &cartridgeInfoFileInfo{}, nil
}

type cartridgeInfoFileInfo struct{}

func (fi *cartridgeInfoFileInfo) Name() string       { return "info" }
func (fi *cartridgeInfoFileInfo) Size() int64        { return 0 } // Dynamic size
func (fi *cartridgeInfoFileInfo) Mode() fs.FileMode  { return 0444 } // Read-only
func (fi *cartridgeInfoFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *cartridgeInfoFileInfo) IsDir() bool        { return false }
func (fi *cartridgeInfoFileInfo) Sys() interface{}   { return nil }

// decodeCartridgeType maps cartridge type byte to MBC name and battery flag
func decodeCartridgeType(typeByte byte) (string, bool) {
	switch typeByte {
	case 0x00:
		return "ROM", false
	case 0x01:
		return "MBC1", false
	case 0x02:
		return "MBC1", false // MBC1+RAM
	case 0x03:
		return "MBC1", true // MBC1+RAM+BATTERY
	case 0x05:
		return "MBC2", false
	case 0x06:
		return "MBC2", true // MBC2+BATTERY
	case 0x0F:
		return "MBC3", true // MBC3+TIMER+BATTERY
	case 0x10:
		return "MBC3", true // MBC3+TIMER+RAM+BATTERY
	case 0x11:
		return "MBC3", false
	case 0x12:
		return "MBC3", false // MBC3+RAM
	case 0x13:
		return "MBC3", true // MBC3+RAM+BATTERY
	case 0x19:
		return "MBC5", false
	case 0x1A:
		return "MBC5", false // MBC5+RAM
	case 0x1B:
		return "MBC5", true // MBC5+RAM+BATTERY
	case 0x1C:
		return "MBC5", false // MBC5+RUMBLE
	case 0x1D:
		return "MBC5", false // MBC5+RUMBLE+RAM
	case 0x1E:
		return "MBC5", true // MBC5+RUMBLE+RAM+BATTERY
	default:
		return "UNKNOWN", false
	}
}

func boolToHex(b bool) byte {
	if b {
		return 0x01
	}
	return 0x00
}
