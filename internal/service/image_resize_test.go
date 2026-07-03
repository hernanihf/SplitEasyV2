package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func makeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestResizeReceiptImage_PassesPDFThroughUnchanged(t *testing.T) {
	original := []byte("%PDF-1.4 fake pdf bytes")
	data, mimeType := ResizeReceiptImage(original, "application/pdf")
	if mimeType != "application/pdf" {
		t.Errorf("expected mime type to stay application/pdf, got %q", mimeType)
	}
	if !bytes.Equal(data, original) {
		t.Error("expected PDF bytes to be returned unchanged")
	}
}

func TestResizeReceiptImage_FallsBackOnUndecodableData(t *testing.T) {
	original := []byte("not actually an image")
	data, mimeType := ResizeReceiptImage(original, "image/jpeg")
	if mimeType != "image/jpeg" {
		t.Errorf("expected mime type to stay image/jpeg on decode failure, got %q", mimeType)
	}
	if !bytes.Equal(data, original) {
		t.Error("expected original bytes to be returned on decode failure")
	}
}

func TestResizeReceiptImage_DownscalesOversizedImage(t *testing.T) {
	original := makeTestPNG(t, 2000, 1000)

	data, mimeType := ResizeReceiptImage(original, "image/png")
	if mimeType != "image/jpeg" {
		t.Errorf("expected re-encoding to image/jpeg, got %q", mimeType)
	}

	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected resized output to decode as jpeg: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != maxReceiptImageDimension {
		t.Errorf("expected longest edge to be capped at %d, got %d", maxReceiptImageDimension, bounds.Dx())
	}
	if bounds.Dy() != 800 {
		t.Errorf("expected height to scale proportionally to 800, got %d", bounds.Dy())
	}
}

func TestResizeReceiptImage_LeavesSmallImageDimensionsUntouched(t *testing.T) {
	original := makeTestPNG(t, 100, 50)

	data, mimeType := ResizeReceiptImage(original, "image/png")
	if mimeType != "image/jpeg" {
		t.Errorf("expected re-encoding to image/jpeg even for a small image, got %q", mimeType)
	}

	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("expected output to decode as jpeg: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != 100 || bounds.Dy() != 50 {
		t.Errorf("expected dimensions to stay 100x50, got %dx%d", bounds.Dx(), bounds.Dy())
	}
}
