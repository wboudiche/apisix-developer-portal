package admin

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// a 32x32 red WebP (Go's x/image can decode but not encode WebP).
const redWebP32 = "UklGRkoAAABXRUJQVlA4ID4AAAAwAwCdASogACAAPm00lkekIyIhKAgAgA2JZQDMSoAAQFBQAP7vKUf43m81s4//7B3/6Dv/0Hf7Jtvb2AAAAA=="

func rasterPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{200, 40, 40, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func rasterJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDecodeAndReencodeAcceptsRaster(t *testing.T) {
	webp, _ := base64.StdEncoding.DecodeString(redWebP32)
	for name, raw := range map[string][]byte{
		"png":  rasterPNG(t, 64, 64),
		"jpeg": rasterJPEG(t, 64, 64),
		"webp": webp,
	} {
		out, err := DecodeAndReencode(raw)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", name, err)
		}
		if _, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "png" {
			t.Fatalf("%s: output not PNG (format=%q err=%v)", name, format, err)
		}
	}
}

func TestDecodeAndReencodeRejectsNonRaster(t *testing.T) {
	cases := map[string][]byte{
		"svg":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"text":       []byte("this is definitely not an image, padded out to beyond 512 bytes " + strings.Repeat("x", 600)),
		"fake-magic": append([]byte("\x89PNG\r\n\x1a\n"), []byte(strings.Repeat("x", 600))...),
	}
	for name, raw := range cases {
		if _, err := DecodeAndReencode(raw); err == nil {
			t.Fatalf("%s: expected rejection, got nil", name)
		}
	}
}

func TestDecodeAndReencodeRejectsOversizeDimensions(t *testing.T) {
	if _, err := DecodeAndReencode(rasterPNG(t, 600, 600)); err == nil {
		t.Fatal("600x600: expected ErrIconUndecodable, got nil")
	}
	if _, err := DecodeAndReencode(rasterPNG(t, 8, 8)); err == nil {
		t.Fatal("8x8: expected rejection (below min), got nil")
	}
}

func TestDecodeAndReencodeStripsTrailingBytes(t *testing.T) {
	polyglot := append(rasterPNG(t, 64, 64), []byte("<?php evil(); ?>")...)
	out, err := DecodeAndReencode(polyglot)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if bytes.Contains(out, []byte("evil")) {
		t.Fatal("re-encoded output still contains trailing payload")
	}
}
