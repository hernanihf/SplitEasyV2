package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"spliteasy/internal/domain"
	"spliteasy/internal/service"
)

type fakeReceiptService struct {
	gotMimeType string
	gotCurrency string
	err         error
}

func (f *fakeReceiptService) ParseReceipt(_ context.Context, imageBytes []byte, mimeType, currency string) (*domain.ReceiptScan, error) {
	f.gotMimeType = mimeType
	f.gotCurrency = currency
	if f.err != nil {
		return nil, f.err
	}
	return &domain.ReceiptScan{}, nil
}

// newMultipartRequest builds a scan request with an explicit (and possibly
// spoofed) Content-Type on the file part, to make sure the handler doesn't
// trust it.
func newMultipartRequest(t *testing.T, filename, declaredContentType string, content []byte) *http.Request {
	t.Helper()
	return newMultipartRequestWithCurrency(t, filename, declaredContentType, content, "")
}

// newMultipartRequestWithCurrency is newMultipartRequest plus an optional
// "currency" form field, for tests that need to verify it's threaded through.
func newMultipartRequestWithCurrency(t *testing.T, filename, declaredContentType string, content []byte, currency string) *http.Request {
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
	if currency != "" {
		if err := w.WriteField("currency", currency); err != nil {
			t.Fatalf("write currency field: %v", err)
		}
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

func TestScanReceipt_ThreadsCurrencyToService(t *testing.T) {
	fake := &fakeReceiptService{}
	h := NewReceiptHandler(fake)
	req := newMultipartRequestWithCurrency(t, "receipt.jpg", "image/jpeg", []byte("fake-bytes"), "ARS")

	rec := httptest.NewRecorder()
	h.ScanReceipt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.gotCurrency != "ARS" {
		t.Errorf("expected currency %q to reach the service, got %q", "ARS", fake.gotCurrency)
	}
}

func TestScanReceipt_MapsServiceErrorsToStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string // empty means "don't check exact body"
	}{
		{"unavailable", service.ErrReceiptScanningUnavailable, http.StatusServiceUnavailable, ""},
		{"empty image", service.ErrReceiptImageEmpty, http.StatusBadRequest, ""},
		{"too large", fmt.Errorf("%w (max 4 bytes)", service.ErrReceiptFileTooLarge), http.StatusBadRequest, ""},
		{"unsupported type", fmt.Errorf("%w: %q", service.ErrReceiptUnsupportedType, "text/plain"), http.StatusBadRequest, ""},
		{
			"upstream/internal failure",
			errors.New("anthropic API returned status 529: overloaded, retry with backoff"),
			http.StatusInternalServerError,
			"internal server error\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeReceiptService{err: tc.err}
			h := NewReceiptHandler(fake)
			req := newMultipartRequest(t, "receipt.jpg", "image/jpeg", []byte("fake-bytes"))

			rec := httptest.NewRecorder()
			h.ScanReceipt(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
				t.Fatalf("expected generic body %q, got %q", tc.wantBody, rec.Body.String())
			}
			if tc.wantStatus == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "anthropic") {
				t.Fatalf("internal error detail leaked to client: %s", rec.Body.String())
			}
		})
	}
}
