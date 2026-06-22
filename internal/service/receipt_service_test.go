package service

import (
	"bytes"
	"io"
	"net/http"
	"testing"
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

	_, err := svc.ParseReceipt([]byte("fake-image-bytes"), "application/pdf")
	if err == nil {
		t.Error("expected error for unsupported mime type")
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

func TestParseReceipt_ReturnsErrorOnNonOKStatus(t *testing.T) {
	body := `{"error":{"message":"invalid api key"}}`
	doer := &fakeHTTPDoer{response: jsonResponse(http.StatusUnauthorized, body)}
	svc := NewReceiptService(doer, "bad-key", "claude-3-5-sonnet-20241022")

	_, err := svc.ParseReceipt([]byte("fake-image-bytes"), "image/jpeg")
	if err == nil {
		t.Error("expected error on non-200 status")
	}
}
