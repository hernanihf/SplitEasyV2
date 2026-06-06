package handler

import (
	"encoding/json"
	"net/http"
	"spliteasy/internal/service"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type UserHandler struct {
	userService service.UserService
}

func NewUserHandler(userService service.UserService) *UserHandler {
	return &UserHandler{userService}
}

type CreateUserRequest struct {
	Name  string `json:"name" example:"John Doe"`
	Email string `json:"email" example:"john@example.com"`
}

// CreateUser godoc
// @Summary      Create a user
// @Description  Creates a new user in the system.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        user  body      CreateUserRequest  true  "User Info"
// @Success      201   {object}  domain.User
// @Failure      400   {string}  string  "Bad Request"
// @Failure      401   {string}  string  "Unauthorized"
// @Failure      500   {string}  string  "Internal Server Error"
// @Security     JWT
// @Router       /users [post]
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := h.userService.CreateUser(req.Name, req.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// GetUser godoc
// @Summary      Get a user
// @Description  Retrieves a user by ID.
// @Tags         users
// @Produce      json
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  domain.User
// @Failure      400  {string}  string  "Bad Request"
// @Failure      401  {string}  string  "Unauthorized"
// @Failure      404  {string}  string  "Not Found"
// @Security     JWT
// @Router       /users/{id} [get]
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	user, err := h.userService.GetUser(uint(id))
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
