package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"spliteasy/internal/domain"
)

const (
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"

	// MaxReceiptImageBytes caps the raw (pre-base64) upload size. Anthropic
	// limits each image to 5MB once base64-encoded (~33% overhead), so we
	// stay comfortably under that.
	MaxReceiptImageBytes = 4 * 1024 * 1024
)

// Sentinel errors for the input-validation failures a caller can act on —
// the handler maps these to specific status codes. Anything else ParseReceipt
// returns (a failed Anthropic call, a decode error, ...) is our fault, not
// the request's, and the handler treats it as a generic 500 instead.
var (
	// ErrReceiptScanningUnavailable means the service isn't configured (e.g.
	// missing ANTHROPIC_API_KEY) — not the caller's fault, but also not
	// something to explain in detail to an API consumer.
	ErrReceiptScanningUnavailable = errors.New("receipt scanning is currently unavailable")
	ErrReceiptImageEmpty          = errors.New("image is empty")
	ErrReceiptFileTooLarge        = errors.New("file is too large")
	ErrReceiptUnsupportedType     = errors.New("unsupported file type")
)

// receiptPrompt embeds the fixed category list so the model's suggestion is
// always one of the slugs the rest of the system accepts.
var receiptPrompt = `You are extracting structured data from a store receipt, ticket or invoice (provided as an image or PDF). Respond with ONLY a single JSON object (no markdown fences, no explanation) matching exactly this shape:
{"merchant_name": string, "date": string (ISO 8601 "YYYY-MM-DD" if found, else ""), "total_amount": number, "category": string, "items": [{"description": string, "price": number}]}
"merchant_name" and every item's "description" must be transcribed exactly as printed on the receipt, in its original language — do not translate, paraphrase, or normalize them, even though these instructions are in English.
"category" must be exactly one of: ` + strings.Join(domain.ExpenseCategorySlugs, ", ") + `. Pick the best fit for the merchant/purchase (e.g. a restaurant receipt is "food", a supermarket is "groceries", a gas station is "fuel"); use "other" only if nothing fits.
If a field cannot be determined, use an empty string or 0. Amounts must be plain numbers without currency symbols.`

var supportedReceiptMimeTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"image/gif":       true,
	"application/pdf": true,
}

// contentBlockTypeFor maps an upload's MIME type to the Anthropic content block
// kind: PDFs go in a "document" block, everything else in an "image" block.
func contentBlockTypeFor(mimeType string) string {
	if mimeType == "application/pdf" {
		return "document"
	}
	return "image"
}

// httpDoer is satisfied by *http.Client; kept as an interface so tests can
// inject a fake without making real network calls.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type ReceiptService interface {
	ParseReceipt(ctx context.Context, imageBytes []byte, mimeType string) (*domain.ReceiptScan, error)
}

type receiptService struct {
	httpClient     httpDoer
	apiKey         string
	model          string
	storageService StorageService // nil when Supabase Storage isn't configured
}

func NewReceiptService(httpClient httpDoer, apiKey, model string, storageService StorageService) ReceiptService {
	return &receiptService{httpClient, apiKey, model, storageService}
}

// receiptStoragePath returns a fresh, unguessable object key for a scanned
// receipt — same random-token approach as generateInviteToken (group_service.go)
// rather than pulling in a UUID dependency for one call site.
func receiptStoragePath(mimeType string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b) + "." + fileExtensionFor(mimeType), nil
}

func fileExtensionFor(mimeType string) string {
	switch mimeType {
	case "application/pdf":
		return "pdf"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "jpg"
	}
}

// anthropicSource is the base64 payload shared by "image" and "document" blocks.
type anthropicSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicContentBlock struct {
	Type   string           `json:"type"`
	Text   string           `json:"text,omitempty"`
	Source *anthropicSource `json:"source,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (s *receiptService) ParseReceipt(ctx context.Context, imageBytes []byte, mimeType string) (*domain.ReceiptScan, error) {
	if s.apiKey == "" {
		slog.Error("receipt scan requested but ANTHROPIC_API_KEY is not configured")
		return nil, ErrReceiptScanningUnavailable
	}
	if len(imageBytes) == 0 {
		return nil, ErrReceiptImageEmpty
	}
	if len(imageBytes) > MaxReceiptImageBytes {
		return nil, fmt.Errorf("%w (max %d bytes)", ErrReceiptFileTooLarge, MaxReceiptImageBytes)
	}
	if !supportedReceiptMimeTypes[mimeType] {
		return nil, fmt.Errorf("%w: %q", ErrReceiptUnsupportedType, mimeType)
	}

	reqBody := anthropicRequest{
		Model:     s.model,
		MaxTokens: 1024,
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: []anthropicContentBlock{
					{
						Type: contentBlockTypeFor(mimeType),
						Source: &anthropicSource{
							Type:      "base64",
							MediaType: mimeType,
							Data:      base64.StdEncoding.EncodeToString(imageBytes),
						},
					},
					{Type: "text", Text: receiptPrompt},
				},
			},
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", s.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	httpResp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer httpResp.Body.Close()

	var apiResp anthropicResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode anthropic response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		if apiResp.Error != nil {
			return nil, fmt.Errorf("anthropic API error: %s", apiResp.Error.Message)
		}
		return nil, fmt.Errorf("anthropic API returned status %d", httpResp.StatusCode)
	}

	if len(apiResp.Content) == 0 {
		return nil, errors.New("anthropic response had no content")
	}

	rawJSON := strings.TrimSpace(apiResp.Content[0].Text)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var scan domain.ReceiptScan
	if err := json.Unmarshal([]byte(rawJSON), &scan); err != nil {
		return nil, fmt.Errorf("failed to parse receipt data from model response: %w", err)
	}

	// The prompt constrains the category to the fixed list, but the model
	// output is still untrusted — coerce anything else to the default rather
	// than letting a made-up slug flow into the expense form.
	if !domain.IsValidExpenseCategory(scan.Category) {
		scan.Category = domain.DefaultExpenseCategory
	}

	// Persisting the image is additive: if storage isn't configured, or the
	// upload fails, the scan itself is still fully valid and useful — log and
	// move on rather than failing a request whose OCR already succeeded.
	if s.storageService != nil {
		resizedBytes, resizedMime := ResizeReceiptImage(imageBytes, mimeType)
		path, err := receiptStoragePath(resizedMime)
		if err != nil {
			slog.Error("failed to generate receipt storage path", "error", err)
		} else if err := s.storageService.Upload(ctx, path, resizedBytes, resizedMime); err != nil {
			slog.Error("failed to upload receipt image", "error", err)
		} else {
			scan.ReceiptImagePath = path
		}
	}

	return &scan, nil
}
