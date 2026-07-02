package service

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"spliteasy/internal/domain"
)

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
	svc := NewReceiptService(&fakeHTTPDoer{}, "", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
	if err == nil {
		t.Error("expected error when API key is not configured")
	}
}

func TestParseReceipt_RejectsEmptyImage(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte{}, "image/jpeg")
	if err == nil {
		t.Error("expected error for empty image")
	}
}

func TestParseReceipt_RejectsUnsupportedMimeType(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte("fake-bytes"), "text/plain")
	if err == nil {
		t.Error("expected error for unsupported mime type")
	}
}

func TestParseReceipt_AcceptsPDFAsDocumentBlock(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Acme\",\"date\":\"\",\"total_amount\":0,\"items\":[]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte("fake-pdf-bytes"), "application/pdf")
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

func TestParseReceipt_RejectsOversizedImage(t *testing.T) {
	svc := NewReceiptService(&fakeHTTPDoer{}, "test-key", "claude-3-5-sonnet-20241022")

	tooLarge := make([]byte, MaxReceiptImageBytes+1)
	_, err := svc.ParseReceipt(tooLarge, "image/jpeg")
	if err == nil {
		t.Error("expected error for oversized image")
	}
}

func TestParseReceipt_ParsesSuccessfulResponse(t *testing.T) {
	body := `{"content":[{"type":"text","text":"{\"merchant_name\":\"Supermercado\",\"date\":\"2026-06-21\",\"total_amount\":1500.50,\"items\":[{\"description\":\"Pan\",\"price\":500},{\"description\":\"Leche\",\"price\":1000.50}]}"}]}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusOK, body)}
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022")

	scan, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
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
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022")

	scan, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/png")
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
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022")

	scan, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
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
	svc := NewReceiptService(doer, "test-key", "claude-3-5-sonnet-20241022")

	scan, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
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
	svc := NewReceiptService(doer, "bad-key", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
	if err == nil {
		t.Error("expected error on non-200 status")
	}
}
