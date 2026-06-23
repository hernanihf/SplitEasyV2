package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"spliteasy/internal/config"
	"spliteasy/internal/handler/middleware"
	"spliteasy/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type GroupHandler struct {
	groupService service.GroupService
}

func NewGroupHandler(groupService service.GroupService) *GroupHandler {
	return &GroupHandler{groupService}
}

type CreateGroupRequest struct {
	Name string `json:"name" example:"Trip to Paris"`
}

// CreateGroup godoc
// @Summary      Create a group
// @Description  Creates a new group for sharing expenses. The authenticated user becomes its creator and first member.
// @Tags         groups
// @Accept       json
// @Produce      json
// @Param        group  body      CreateGroupRequest  true  "Group Info"
// @Success      201    {object}  domain.Group
// @Failure      400    {string}  string  "Bad Request"
// @Failure      401    {string}  string  "Unauthorized"
// @Failure      500    {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups [post]
func (h *GroupHandler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	group, err := h.groupService.CreateGroup(req.Name, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(group)
}

// GetGroup godoc
// @Summary      Get a group
// @Description  Retrieves a group by ID.
// @Tags         groups
// @Produce      json
// @Param        id   path      int  true  "Group ID"
// @Success      200  {object}  domain.Group
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      404  {string}  string  "Not Found"
// @Security     JWT
// @Router       /groups/{id} [get]
func (h *GroupHandler) GetGroup(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	group, err := h.groupService.GetGroup(uint(id))
	if err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

// ListGroups godoc
// @Summary      List groups for the authenticated user
// @Description  Retrieves all groups the authenticated user is a member of.
// @Tags         groups
// @Produce      json
// @Success      200  {array}   domain.Group
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      500  {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /groups [get]
func (h *GroupHandler) ListGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	groups, err := h.groupService.ListGroupsForUser(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

type InviteResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// frontendJoinURL builds the shareable join link from the configured frontend
// origin (derived from FRONTEND_REDIRECT_URL) plus the invite token.
func frontendJoinURL(token string) string {
	base, err := url.Parse(config.FrontendRedirectURL)
	if err != nil {
		return "/join/" + token
	}
	return base.Scheme + "://" + base.Host + "/join/" + token
}

// GetInvite godoc
// @Summary      Get a group's invite link
// @Description  Returns the share token and a join link for the group. Only group members can request it.
// @Tags         groups
// @Produce      json
// @Param        id   path      int  true  "Group ID"
// @Success      200  {object}  handler.InviteResponse
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      403  {string}  string  "Forbidden"
// @Security     JWT
// @Router       /groups/{id}/invite [get]
func (h *GroupHandler) GetInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	token, err := h.groupService.GetInviteToken(uint(id), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(InviteResponse{Token: token, URL: frontendJoinURL(token)})
}

type JoinGroupRequest struct {
	Token string `json:"token" example:"a1b2c3d4..."`
}

// JoinGroup godoc
// @Summary      Join a group via invite token
// @Description  Adds the authenticated user to the group identified by the invite token. Idempotent.
// @Tags         groups
// @Accept       json
// @Produce      json
// @Param        invite  body      JoinGroupRequest  true  "Invite token"
// @Success      200     {object}  domain.Group
// @Failure      400     {string}  string  "Bad Request"
// @Failure      401     {string}  string  "Unauthorized"
// @Security     JWT
// @Router       /groups/join [post]
func (h *GroupHandler) JoinGroup(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user id in token", http.StatusUnauthorized)
		return
	}

	var req JoinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	group, err := h.groupService.JoinGroup(req.Token, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}
