package handler

import (
	"encoding/json"
	"net/http"
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
	Name      string `json:"name" example:"Trip to Paris"`
	CreatorID uint   `json:"creator_id" example:"1"`
}

// CreateGroup godoc
// @Summary      Create a group
// @Description  Creates a new group for sharing expenses.
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
	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	group, err := h.groupService.CreateGroup(req.Name, req.CreatorID)
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
