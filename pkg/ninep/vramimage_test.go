// ABOUTME: Tests for VRAM tile data to PNG conversion and validation
// ABOUTME: Covers round-trip conversion, dimension validation, and color palette validation
package ninep

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// DMG palette colors for testing
var (
	dmgWhite     = color.RGBA{255, 255, 255, 255}
	dmgLightGray = color.RGBA{170, 170, 170, 255}
	dmgDarkGray  = color.RGBA{85, 85, 85, 255}
	dmgBlack     = color.RGBA{0, 0, 0, 255}
)

func TestDecodeTile(t *testing.T) {
	tests := []struct {
		name     string
		tileData []byte
		expected [][]int
	}{
		{
			name: "all white tile",
			tileData: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			expected: [][]int{
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
			},
		},
		{
			name: "all black tile",
			tileData: []byte{
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			},
			expected: [][]int{
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
			},
		},
		{
			name: "checkerboard pattern",
			tileData: []byte{
				0xAA, 0x00, // 10101010, 00000000 -> 1 0 1 0 1 0 1 0
				0x55, 0x00, // 01010101, 00000000 -> 0 1 0 1 0 1 0 1
				0xAA, 0x00,
				0x55, 0x00,
				0xAA, 0x00,
				0x55, 0x00,
				0xAA, 0x00,
				0x55, 0x00,
			},
			expected: [][]int{
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
				{1, 0, 1, 0, 1, 0, 1, 0},
				{0, 1, 0, 1, 0, 1, 0, 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeTile(tt.tileData, 0)
			if len(result) != 8 {
				t.Fatalf("expected 8 rows, got %d", len(result))
			}
			for y := 0; y < 8; y++ {
				if len(result[y]) != 8 {
					t.Fatalf("row %d: expected 8 pixels, got %d", y, len(result[y]))
				}
				for x := 0; x < 8; x++ {
					if result[y][x] != tt.expected[y][x] {
						t.Errorf("pixel (%d,%d): expected %d, got %d", x, y, tt.expected[y][x], result[y][x])
					}
				}
			}
		})
	}
}

func TestEncodeTile(t *testing.T) {
	tests := []struct {
		name     string
		tile     [][]int
		expected []byte
	}{
		{
			name: "all white tile",
			tile: [][]int{
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
				{0, 0, 0, 0, 0, 0, 0, 0},
			},
			expected: []byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
		},
		{
			name: "all black tile",
			tile: [][]int{
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
				{3, 3, 3, 3, 3, 3, 3, 3},
			},
			expected: []byte{
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeTile(tt.tile)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRoundTripConversion(t *testing.T) {
	// Create test VRAM data with various patterns
	vramSize := 8192 // 8KB for DMG
	vram := make([]byte, vramSize)

	// Fill with test patterns
	for i := 0; i < vramSize; i += 16 {
		// Alternate between different patterns
		pattern := (i / 16) % 4
		switch pattern {
		case 0: // All white
			// Already zeroed
		case 1: // All black
			for j := 0; j < 16; j++ {
				vram[i+j] = 0xFF
			}
		case 2: // Checkerboard
			for j := 0; j < 8; j++ {
				vram[i+j*2] = 0xAA
				vram[i+j*2+1] = 0x00
			}
		case 3: // Gradient
			for j := 0; j < 8; j++ {
				vram[i+j*2] = byte(j * 32)
				vram[i+j*2+1] = 0x00
			}
		}
	}

	// Convert to PNG
	pngData, err := VRAMToPNG(vram)
	if err != nil {
		t.Fatalf("VRAMToPNG failed: %v", err)
	}

	// Verify it's a valid PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("PNG decode failed: %v", err)
	}

	// Check dimensions
	bounds := img.Bounds()
	expectedWidth := 128  // 16 tiles * 8 pixels
	expectedHeight := 256 // 32 tiles * 8 pixels (for 512 tiles / 16 per row)
	if bounds.Dx() != expectedWidth || bounds.Dy() != expectedHeight {
		t.Errorf("expected dimensions %dx%d, got %dx%d", expectedWidth, expectedHeight, bounds.Dx(), bounds.Dy())
	}

	// Convert back to VRAM
	vramResult, err := PNGToVRAM(pngData, vramSize)
	if err != nil {
		t.Fatalf("PNGToVRAM failed: %v", err)
	}

	// Verify round-trip
	if !bytes.Equal(vram, vramResult) {
		t.Error("round-trip conversion failed: VRAM data mismatch")
		// Find first difference for debugging
		for i := 0; i < len(vram); i++ {
			if vram[i] != vramResult[i] {
				t.Errorf("first difference at byte %d: expected %02x, got %02x", i, vram[i], vramResult[i])
				break
			}
		}
	}
}

func TestPNGValidation_Dimensions(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		height      int
		expectedErr string
	}{
		{
			name:        "wrong width",
			width:       127,
			height:      256,
			expectedErr: "invalid dimensions",
		},
		{
			name:        "wrong height",
			width:       128,
			height:      255,
			expectedErr: "invalid dimensions",
		},
		{
			name:        "completely wrong",
			width:       256,
			height:      256,
			expectedErr: "invalid dimensions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a PNG with wrong dimensions
			img := image.NewRGBA(image.Rect(0, 0, tt.width, tt.height))
			for y := 0; y < tt.height; y++ {
				for x := 0; x < tt.width; x++ {
					img.Set(x, y, dmgWhite)
				}
			}

			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				t.Fatalf("failed to encode test PNG: %v", err)
			}

			// Try to convert - should fail
			_, err := PNGToVRAM(buf.Bytes(), 8192)
			if err == nil {
				t.Error("expected error for invalid dimensions, got nil")
			} else if !bytes.Contains([]byte(err.Error()), []byte(tt.expectedErr)) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestPNGValidation_Colors(t *testing.T) {
	tests := []struct {
		name        string
		color       color.RGBA
		expectedErr string
	}{
		{
			name:        "off-by-one white",
			color:       color.RGBA{255, 255, 254, 255},
			expectedErr: "invalid color",
		},
		{
			name:        "random gray",
			color:       color.RGBA{100, 100, 100, 255},
			expectedErr: "invalid color",
		},
		{
			name:        "transparent pixel",
			color:       color.RGBA{255, 255, 255, 128},
			expectedErr: "invalid color",
		},
		{
			name:        "red pixel",
			color:       color.RGBA{255, 0, 0, 255},
			expectedErr: "invalid color",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create valid-sized PNG with invalid color
			img := image.NewRGBA(image.Rect(0, 0, 128, 256))
			for y := 0; y < 256; y++ {
				for x := 0; x < 128; x++ {
					img.Set(x, y, dmgWhite)
				}
			}
			// Set one invalid pixel
			img.Set(0, 0, tt.color)

			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				t.Fatalf("failed to encode test PNG: %v", err)
			}

			// Try to convert - should fail
			_, err := PNGToVRAM(buf.Bytes(), 8192)
			if err == nil {
				t.Error("expected error for invalid color, got nil")
			} else if !bytes.Contains([]byte(err.Error()), []byte(tt.expectedErr)) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestPNGValidation_ValidColors(t *testing.T) {
	// Create PNG with all valid DMG colors
	img := image.NewRGBA(image.Rect(0, 0, 128, 256))
	colors := []color.RGBA{dmgWhite, dmgLightGray, dmgDarkGray, dmgBlack}

	for y := 0; y < 256; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, colors[(x+y)%4])
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test PNG: %v", err)
	}

	// Should succeed
	_, err := PNGToVRAM(buf.Bytes(), 8192)
	if err != nil {
		t.Errorf("expected success with valid colors, got error: %v", err)
	}
}

func TestVRAMToPNG_16KB(t *testing.T) {
	// Test with 16KB VRAM (CGB)
	vramSize := 16384
	vram := make([]byte, vramSize)

	// Fill with test pattern
	for i := 0; i < vramSize; i++ {
		vram[i] = byte(i % 256)
	}

	pngData, err := VRAMToPNG(vram)
	if err != nil {
		t.Fatalf("VRAMToPNG failed: %v", err)
	}

	// Decode and check dimensions
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		t.Fatalf("PNG decode failed: %v", err)
	}

	bounds := img.Bounds()
	expectedWidth := 128
	expectedHeight := 512 // 64 tiles tall for 16KB
	if bounds.Dx() != expectedWidth || bounds.Dy() != expectedHeight {
		t.Errorf("expected dimensions %dx%d, got %dx%d", expectedWidth, expectedHeight, bounds.Dx(), bounds.Dy())
	}
}
