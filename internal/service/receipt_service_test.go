package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"spliteasy/internal/domain"
)

// fakeStorageService lets receipt_service tests observe/control the upload
// step without a real Supabase Storage instance.
type fakeStorageService struct {
	uploadErr    error
	uploadedPath string
	uploadedData []byte
	deleteErr    error
	deletedPaths []string
}

func (f *fakeStorageService) Upload(_ context.Context, path string, data []byte, _ string) error {
	f.uploadedPath = path
	f.uploadedData = data
	return f.uploadErr
}

func (f *fakeStorageService) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("not used in these tests")
}

func (f *fakeStorageService) Delete(_ context.Context, path string) error {
	f.deletedPaths = append(f.deletedPaths, path)
	return f.deleteErr
}

type fakeHTTPDoer struct {
	response *http.Response
	err      error
	lastReq  *http.Request
}

func (f *fakeHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	return f.response, f.err
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestParseReceipt_RejectsMissingAPIKey(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "", "claude-3-5-sonnet-20241022", nil)

	_, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if !errors.Is(err, ErrReceiptScanningUnavailable) {
		t.Errorf("expected ErrReceiptScanningUnavailable, got %v", err)
	}
}

func TestParseReceipt_RejectsEmptyImage(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022", nil)

	_, err := svc.ParseReceipt(context.Background(), []byte{}, "image/jpeg", "")
	if !errors.Is(err, ErrReceiptImageEmpty) {
		t.Errorf("expected ErrReceiptImageEmpty, got %v", err)
	}
}

func TestParseReceipt_RejectsUnsupportedMimeType(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022", nil)

	_, err := svc.ParseReceipt(context.Background(), []byte("fake-bytes"), "text/plain", "")
	if !errors.Is(err, ErrReceiptUnsupportedType) {
		t.Errorf("expected ErrReceiptUnsupportedType, got %v", err)
	}
}

func TestParseReceipt_AcceptsPDFAsDocumentBlock(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Acme\",\"date\":\"\",\"total_amount\":0,\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

	_, err := svc.ParseReceipt(context.Background(), []byte("fake-pdf-bytes"), "application/pdf", "")
	if err != nil {
		t.Fatalf("unexpected error scanning a PDF: %v", err)
	}

	sentBody, _ := io.ReadAll(doer.lastReq.Body)
	if !bytes.Contains(sentBody, []byte(`"type":"document"`)) {
		t.Errorf("expected a document content block for a PDF, got: %s", sentBody)
	}
	if !bytes.Contains(sentBody, []byte(`"media_type":"application/pdf"`)) {
		t.Errorf("expected media_type application/pdf in the request, got: %s", sentBody)
	}
}

func TestParseReceipt_NumberFormatHintMatchesGroupCurrency(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Acme\",\"date\":\"\",\"total_amount\":0,\"items\":[]}"}]}`

	cases := []struct {
		name     string
		currency string
		want     string
	}{
		{"comma-decimal currency", "ARS", "comma as the decimal separator"},
		{"period-decimal currency", "USD", "period as the decimal separator"},
		{"empty currency falls back to default (ARS, comma-decimal)", "", "comma as the decimal separator"},
		{"invalid currency falls back to default (ARS, comma-decimal)", "XYZ", "comma as the decimal separator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
			svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

			_, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", tc.currency)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			sentBody, _ := io.ReadAll(doer.lastReq.Body)
			if !bytes.Contains(sentBody, []byte(tc.want)) {
				t.Errorf("expected prompt to contain %q for currency %q, got: %s", tc.want, tc.currency, sentBody)
			}
		})
	}
}

func TestParseReceipt_RejectsOversizedImage(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022", nil)

	tooLarge := make([]byte, MaxReceiptImageBytes+1)
	_, err := svc.ParseReceipt(context.Background(), tooLarge, "image/jpeg", "")
	if !errors.Is(err, ErrReceiptFileTooLarge) {
		t.Errorf("expected ErrReceiptFileTooLarge, got %v", err)
	}
}

func TestParseReceipt_ParsesSuccessfulResponse(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Supermercado\",\"date\":\"2026-06-21\",\"total_amount\":1500.50,\"items\":[{\"description\":\"Pan\",\"price\":500},{\"description\":\"Leche\",\"price\":1000.50}]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.MerchantName != "Supermercado" {
		t.Errorf("expected merchant_name 'Supermercado', got %q", scan.MerchantName)
	}
	if scan.TotalAmount != 1500.50 {
		t.Errorf("expected total_amount 1500.50, got %v", scan.TotalAmount)
	}
	if len(scan.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(scan.Items))
	}

	if doer.lastReq.Header.Get("x-api-key") != "test-key" {
		t.Error("expected x-api-key header to be set")
	}
}

func TestParseReceipt_StripsMarkdownFences(t *testing.T) {
	body := "{\"content\":[{\"type\":\"text\",\"text\":\"```json\\n{\\\"merchant_name\\\":\\\"Kiosco\\\",\\\"date\\\":\\\"\\\",\\\"total_amount\\\":100,\\\"items\\\":[]}\\n```\"}]}"
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/png", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.MerchantName != "Kiosco" {
		t.Errorf("expected merchant_name 'Kiosco', got %q", scan.MerchantName)
	}
}

func TestParseReceipt_KeepsValidSuggestedCategory(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Supermercado\",\"date\":\"\",\"total_amount\":100,\"category\":\"groceries\",\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.Category != "groceries" {
		t.Errorf("expected category 'groceries', got %q", scan.Category)
	}
}

func TestParseReceipt_CoercesUnknownCategoryToDefault(t *testing.T) {
	// The model output is untrusted — a slug outside the fixed list (or a
	// missing one) must fall back to the default, never flow through as-is.
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Tienda\",\"date\":\"\",\"total_amount\":100,\"category\":\"yachts\",\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", nil)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.Category != domain.DefaultExpenseCategory {
		t.Errorf("expected category to be coerced to %q, got %q", domain.DefaultExpenseCategory, scan.Category)
	}
}

func TestParseReceipt_ReturnsErrorOnNonOKStatus(t *testing.T) {
	body := `{"error":{"message":"invalid api key"}}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusUnauthorized, body)}
	svc := NewReceiptService(doer, "bad-key", "claude-3-5-sonnet-20241022", nil)

	_, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err == nil {
		t.Error("expected error on non-200 status")
	}
}

func TestParseReceipt_UploadsImageAndSetsReceiptImagePath(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Kiosco\",\"date\":\"\",\"total_amount\":100,\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	storage := &fakeStorageService{}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", storage)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.ReceiptImagePath == "" {
		t.Error("expected ReceiptImagePath to be set after a successful upload")
	}
	if storage.uploadedPath != scan.ReceiptImagePath {
		t.Errorf("expected the uploaded path to match scan.ReceiptImagePath, got %q vs %q", storage.uploadedPath, scan.ReceiptImagePath)
	}
	if len(storage.uploadedData) == 0 {
		t.Error("expected non-empty uploaded data")
	}
}

func TestParseReceipt_SucceedsWithoutImagePathWhenUploadFails(t *testing.T) {
	// The scan itself is already useful even if persisting the image fails —
	// this must not turn a successful OCR into a request error.
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Kiosco\",\"date\":\"\",\"total_amount\":100,\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	storage := &fakeStorageService{uploadErr: errors.New("network error")}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022", storage)

	scan, err := svc.ParseReceipt(context.Background(), []byte("fake-image-bytes"), "image/jpeg", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scan.MerchantName != "Kiosco" {
		t.Errorf("expected the scan to still succeed, got merchant %q", scan.MerchantName)
	}
	if scan.ReceiptImagePath != "" {
		t.Error("expected no ReceiptImagePath when the upload fails")
	}
}
