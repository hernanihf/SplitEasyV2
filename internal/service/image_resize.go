package service

import (
	"bytes"
	"image"
	"image/jpeg"

	// Registered so image.Decode recognizes PNG/GIF/WEBP source files (the
	// other formats receipt_service.go's supportedReceiptMimeTypes allows).
	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
)

const (
	maxReceiptImageDimension = 1600
	receiptJPEGQuality       = 75
)

// ResizeReceiptImage shrinks a receipt photo to a reasonable storage size:
// capped at maxReceiptImageDimension on the longer edge, re-encoded as JPEG
// at receiptJPEGQuality. This is a storage-cost optimization, not a
// correctness requirement — on any failure (unrecognized format, decode
// error, a WEBP the stdlib can't read, etc.) it returns the original bytes
// and mimeType unchanged rather than erroring, so a hiccup here never blocks
// saving the receipt. PDFs pass through untouched (no attempt to rasterize
// them — that needs a much heavier dependency for little benefit, since PDF
// receipts are typically already compact).
func ResizeReceiptImage(data []byte, mimeType string) ([]byte, string) {
	if mimeType == "application/pdf" {
		return data, mimeType
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, mimeType
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return data, mimeType
	}

	longest := w
	if h > longest {
		longest = h
	}
	if longest > maxReceiptImageDimension {
		scale := float64(maxReceiptImageDimension) / float64(longest)
		newW := int(float64(w) * scale)
		newH := int(float64(h) * scale)
		resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
		draw.CatmullRom.Scale(resized, resized.Bounds(), src, bounds, draw.Over, nil)
		src = resized
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: receiptJPEGQuality}); err != nil {
		return data, mimeType
	}
	return buf.Bytes(), "image/jpeg"
}
