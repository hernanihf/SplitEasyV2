package handler

import (
	"encoding/json"
	"net/http"

	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
)

type PushHandler struct {
	pushService service.PushService
}

func NewPushHandler(pushService service.PushService) *PushHandler {
	return &PushHandler{pushService}
}

type SubscribeRequest struct {
	Endpoint string `json:"endpoint" example:"https://fcm.googleapis.com/fcm/send/..."`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// Subscribe godoc
// @Summary      Register a push subscription
// @Description  Registers (or re-registers) this browser/device for push notifications. Each user can have up to 10 subscriptions.
// @Tags         push
// @Accept       json
// @Param        subscription  body  SubscribeRequest  true  "Push subscription"
// @Success      204  "No Content"
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Security     JWT
// @Router       /push/subscribe [post]
func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		http.Error(w, "endpoint, p256dh and auth are required", http.StatusBadRequest)
		return
	}

	if err := h.pushService.Subscribe(r.Context(), userID, req.Endpoint, req.P256dh, req.Auth); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type UnsubscribeRequest struct {
	Endpoint string `json:"endpoint" example:"https://fcm.googleapis.com/fcm/send/..."`
}

// Unsubscribe godoc
// @Summary      Remove a push subscription
// @Description  Removes this browser/device's push subscription (e.g. when the user disables push and wants the device forgotten).
// @Tags         push
// @Accept       json
// @Param        subscription  body  UnsubscribeRequest  true  "Endpoint to remove"
// @Success      204  "No Content"
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /push/subscribe [delete]
func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req UnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" {
		http.Error(w, "endpoint is required", http.StatusBadRequest)
		return
	}

	if err := h.pushService.Unsubscribe(r.Context(), userID, req.Endpoint); err != nil {
		internalError(w, "failed to remove push subscription", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
