package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

	receiptPrompt = `You are extracting structured data from a store receipt, ticket or invoice (provided as an image or PDF). Respond with ONLY a single JSON object (no markdown fences, no explanation) matching exactly this shape:
{"merchant_name": string, "date": string (ISO 8601 "YYYY-MM-DD" if found, else ""), "total_amount": number, "items": [{"description": string, "price": number}]}
If a field cannot be determined, use an empty string or 0. Amounts must be plain numbers without currency symbols.`
)

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
	ParseReceipt(imageBytes []byte, mimeType string) (*domain.ReceiptScan, error)
}

type receiptService struct {
	httpClient httpDoer
	apiKey     string
	model      string
}

func NewReceiptService(httpClient httpDoer, apiKey, model string) ReceiptService {
	return &receiptService{httpClient, apiKey, model}
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

func (s *receiptService) ParseReceipt(imageBytes []byte, mimeType string) (*domain.ReceiptScan, error) {
	if s.apiKey == "" {
		return nil, errors.New("receipt scanning is not configured (missing ANTHROPIC_API_KEY)")
	}
	if len(imageBytes) == 0 {
		return nil, errors.New("image is empty")
	}
	if len(imageBytes) > MaxReceiptImageBytes {
		return nil, fmt.Errorf("file is too large (max %d bytes)", MaxReceiptImageBytes)
	}
	if !supportedReceiptMimeTypes[mimeType] {
		return nil, fmt.Errorf("unsupported file type %q", mimeType)
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

	httpReq, err := http.NewRequest(http.MethodPost, anthropicAPIURL, bytes.NewReader(payload))
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

	return &scan, nil
}
