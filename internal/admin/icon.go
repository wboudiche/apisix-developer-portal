package admin

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"

	_ "image/jpeg" // register JPEG decoder for image.Decode/DecodeConfig
	_ "image/png"  // register PNG decoder

	_ "golang.org/x/image/webp" // register WebP decoder (decode-only)
)

const (
	iconMaxDim = 512
	iconMinDim = 16
)

// ErrIconType is returned when the bytes are not a supported raster image.
var ErrIconType = errors.New("admin: unsupported icon type")

// ErrIconUndecodable is returned when dimensions are out of range or the image
// cannot be decoded.
var ErrIconUndecodable = errors.New("admin: undecodable icon")

// DecodeAndReencode validates raw as a PNG/JPEG/WebP raster and returns a fresh
// re-encoded PNG. It sniffs the content type (ignoring any caller-supplied
// filename/header), guards dimensions before full decompression, fully decodes
// to prove a real raster, then re-encodes — discarding EXIF, trailing bytes,
// and polyglot tails. SVG and other non-raster inputs are rejected.
func DecodeAndReencode(raw []byte) ([]byte, error) {
	switch http.DetectContentType(raw) {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return nil, ErrIconType
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrIconUndecodable
	}
	if cfg.Width > iconMaxDim || cfg.Height > iconMaxDim || cfg.Width < iconMinDim || cfg.Height < iconMinDim {
		return nil, fmt.Errorf("%w: %dx%d out of [%d,%d]", ErrIconUndecodable, cfg.Width, cfg.Height, iconMinDim, iconMaxDim)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrIconUndecodable
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
