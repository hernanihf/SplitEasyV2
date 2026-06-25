package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"spliteasy/internal/service"
)

type ReceiptHandler struct {
	receiptService service.ReceiptService
}

func NewReceiptHandler(receiptService service.ReceiptService) *ReceiptHandler {
	return &ReceiptHandler{receiptService}
}

// ScanReceipt godoc
// @Summary      Scan a receipt image or PDF
// @Description  Parses a photographed or scanned receipt/ticket/invoice using AI and returns the extracted merchant, date, total and line items, for prefilling a new expense. Accepts both images and PDFs.
// @Tags         expenses
// @Accept       multipart/form-data
// @Produce      json
// @Param        image  formData  file  true  "Receipt file (jpeg, png, webp, gif or pdf)"
// @Success      200    {object}  domain.ReceiptScan
// @Failure      400    {string}  string  "Bad Request"
// @Failure      401    {string}  string  "Unauthorized"
// @Failure      429    {string}  string  "Too Many Requests"
// @Failure      500    {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /receipts/scan [post]
func (h *ReceiptHandler) ScanReceipt(w http.ResponseWriter, r *http.Request) {
	// Cap the whole request body so a malicious upload can't exhaust memory.
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxReceiptImageBytes+1<<20)
	if err := r.ParseMultipartForm(service.MaxReceiptImageBytes + 1<<20); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read image: "+err.Error(), http.StatusBadRequest)
		return
	}

	mimeType := header.Header.Get("Content-Type")

	scan, err := h.receiptService.ParseReceipt(imageBytes, mimeType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(scan)
}
