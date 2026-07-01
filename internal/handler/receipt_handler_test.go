package handler

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"spliteasy/internal/domain"
)

type fakeReceiptService struct {
	gotMimeType string
}

func (f *fakeReceiptService) ParseReceipt(imageBytes []byte, mimeType string) (*domain.ReceiptScan, error) {
	f.gotMimeType = mimeType
	return &domain.ReceiptScan{}, nil
}

// newMultipartRequest builds a scan request with an explicit (and possibly
// spoofed) Content-Type on the file part, to make sure the handler doesn't
// trust it.
func newMultipartRequest(t *testing.T, filename, declaredContentType string, content []byte) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
	partHeader.Set("Content-Type", declaredContentType)
	part, err := w.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/receipts/scan", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestScanReceipt_IgnoresSpoofedContentType(t *testing.T) {
	fake := &fakeReceiptService{}
	h := NewReceiptHandler(fake)

	// Real PNG magic bytes, but the client declares it as image/jpeg.
	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	req := newMultipartRequest(t, "totally-a-photo.jpg", "image/jpeg", pngBytes)

	rec := httptest.NewRecorder()
	h.ScanReceipt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.gotMimeType != "image/png" {
		t.Fatalf("expected the sniffed mime type image/png, got %q", fake.gotMimeType)
	}
}

func TestScanReceipt_DetectsDisguisedExecutable(t *testing.T) {
	fake := &fakeReceiptService{}
	h := NewReceiptHandler(fake)

	// An MZ (Windows executable) header, declared as an image.
	exeBytes := []byte{'M', 'Z', 0x90, 0x00, 0x03, 0x00, 0x00, 0x00}
	req := newMultipartRequest(t, "receipt.jpg", "image/jpeg", exeBytes)

	rec := httptest.NewRecorder()
	h.ScanReceipt(rec, req)

	if fake.gotMimeType == "image/jpeg" {
		t.Fatalf("expected the spoofed image/jpeg to be rejected by sniffing, got %q passed through", fake.gotMimeType)
	}
}
