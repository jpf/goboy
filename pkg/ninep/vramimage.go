// ABOUTME: Converts GameBoy 2bpp tile data to/from PNG format with DMG grayscale palette
// ABOUTME: Provides bidirectional conversion for VRAM visualization and editing
package ninep

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

const (
	tileSize     = 8  // 8x8 pixels per tile
	bytesPerTile = 16 // 2 bytes per row * 8 rows
	tilesPerRow  = 16 // Grid layout: 16 tiles wide
)

// DMG grayscale palette
var dmgColors = [4]color.RGBA{
	{255, 255, 255, 255}, // 0: White
	{170, 170, 170, 255}, // 1: Light gray
	{85, 85, 85, 255},    // 2: Dark gray
	{0, 0, 0, 255},       // 3: Black
}

// VRAMToPNG converts GameBoy VRAM tile data to PNG format.
// VRAM data should be either 8KB (512 tiles) or 16KB (1024 tiles).
// Returns PNG-encoded bytes.
func VRAMToPNG(vramData []byte) ([]byte, error) {
	if len(vramData) != 8192 && len(vramData) != 16384 {
		return nil, fmt.Errorf("invalid VRAM size: %d bytes (expected 8192 or 16384)", len(vramData))
	}

	numTiles := len(vramData) / bytesPerTile
	tiles := make([][][]int, numTiles)

	// Decode all tiles from 2bpp format
	for i := 0; i < numTiles; i++ {
		tiles[i] = decodeTile(vramData, i)
	}

	// Render to image
	img := renderTiles(tiles)

	// Encode as PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("PNG encoding failed: %w", err)
	}

	return buf.Bytes(), nil
}

// PNGToVRAM converts a PNG image back to GameBoy VRAM tile data.
// Validates dimensions and color palette strictly.
// expectedSize should be 8192 or 16384 bytes.
func PNGToVRAM(pngData []byte, expectedSize int) ([]byte, error) {
	// Decode PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("invalid PNG format: %w", err)
	}

	// Validate dimensions
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	expectedWidth := 128
	var expectedHeight int
	switch expectedSize {
	case 8192:
		expectedHeight = 256 // 32 tiles tall
	case 16384:
		expectedHeight = 512 // 64 tiles tall
	default:
		return nil, fmt.Errorf("invalid expected size: %d (must be 8192 or 16384)", expectedSize)
	}

	if width != expectedWidth || height != expectedHeight {
		return nil, fmt.Errorf("invalid dimensions: got %dx%d, expected %dx%d", width, height, expectedWidth, expectedHeight)
	}

	// Extract tiles and validate colors
	numTiles := (height / tileSize) * tilesPerRow
	vram := make([]byte, numTiles*bytesPerTile)

	for tileIdx := 0; tileIdx < numTiles; tileIdx++ {
		gridX := tileIdx % tilesPerRow
		gridY := tileIdx / tilesPerRow
		baseX := gridX * tileSize
		baseY := gridY * tileSize

		// Extract 8x8 tile
		tile := make([][]int, tileSize)
		for y := 0; y < tileSize; y++ {
			tile[y] = make([]int, tileSize)
			for x := 0; x < tileSize; x++ {
				pixelColor := img.At(baseX+x, baseY+y)
				r, g, b, a := pixelColor.RGBA()

				// Convert from uint32 (16-bit) to uint8
				r8 := uint8(r >> 8)
				g8 := uint8(g >> 8)
				b8 := uint8(b >> 8)
				a8 := uint8(a >> 8)

				// Validate color is in DMG palette
				colorValue, ok := rgbaToDMGColor(r8, g8, b8, a8)
				if !ok {
					return nil, fmt.Errorf("invalid color at pixel (%d,%d): RGB(%d,%d,%d,%d) not in DMG palette", baseX+x, baseY+y, r8, g8, b8, a8)
				}
				tile[y][x] = colorValue
			}
		}

		// Encode tile to 2bpp
		tileData := encodeTile(tile)
		copy(vram[tileIdx*bytesPerTile:], tileData)
	}

	return vram, nil
}

// decodeTile extracts one 8x8 tile from 2bpp format to color values (0-3)
func decodeTile(data []byte, tileIndex int) [][]int {
	tile := make([][]int, tileSize)
	offset := tileIndex * bytesPerTile

	for y := 0; y < tileSize; y++ {
		tile[y] = make([]int, tileSize)
		lowByte := data[offset+y*2]
		highByte := data[offset+y*2+1]

		for x := 0; x < tileSize; x++ {
			bit := 7 - x
			low := (lowByte >> bit) & 1
			high := (highByte >> bit) & 1
			tile[y][x] = int((high << 1) | low)
		}
	}

	return tile
}

// encodeTile converts an 8x8 tile from color values (0-3) to 2bpp format
func encodeTile(tile [][]int) []byte {
	data := make([]byte, bytesPerTile)

	for y := 0; y < tileSize; y++ {
		var lowByte, highByte byte
		for x := 0; x < tileSize; x++ {
			bit := 7 - x
			colorValue := tile[y][x]
			low := colorValue & 1
			high := (colorValue >> 1) & 1

			if low != 0 {
				lowByte |= (1 << bit)
			}
			if high != 0 {
				highByte |= (1 << bit)
			}
		}
		data[y*2] = lowByte
		data[y*2+1] = highByte
	}

	return data
}

// renderTiles creates PNG image from tiles (no scaling, 1:1 pixel mapping)
func renderTiles(tiles [][][]int) *image.RGBA {
	numTiles := len(tiles)
	numRows := (numTiles + tilesPerRow - 1) / tilesPerRow

	width := tilesPerRow * tileSize
	height := numRows * tileSize
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for tileIdx, tile := range tiles {
		gridX := tileIdx % tilesPerRow
		gridY := tileIdx / tilesPerRow
		baseX := gridX * tileSize
		baseY := gridY * tileSize

		// Render each pixel of the tile (no scaling)
		for y := 0; y < tileSize; y++ {
			for x := 0; x < tileSize; x++ {
				c := dmgColors[tile[y][x]]
				img.Set(baseX+x, baseY+y, c)
			}
		}
	}

	return img
}

// rgbaToDMGColor converts RGBA values to DMG color index (0-3).
// Returns (colorIndex, true) if valid, (0, false) if not in palette.
func rgbaToDMGColor(r, g, b, a uint8) (int, bool) {
	for i, c := range dmgColors {
		if c.R == r && c.G == g && c.B == b && c.A == a {
			return i, true
		}
	}
	return 0, false
}
