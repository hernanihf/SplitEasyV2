// @title           SplitEasy API
// @version         1.0
// @description     API server for SplitEasy app (Splitwise style expense sharing).
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /api/v1

// @securityDefinitions.apikey  JWT
// @in                          header
// @name                        Authorization
// @description                 Type "Bearer <your-jwt-token>" to authenticate.
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"spliteasy/internal/config"
	"spliteasy/internal/handler"
	mymiddleware "spliteasy/internal/handler/middleware"
	"spliteasy/internal/repository"
	"spliteasy/internal/service"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Initialize Database and Auth Configurations
	config.ConnectDB()
	config.InitAuth()

	// 1. Init Repositories
	userRepo := repository.NewUserRepository(config.DB)
	groupRepo := repository.NewGroupRepository(config.DB)
	expenseRepo := repository.NewExpenseRepository(config.DB)

	// 2. Init Services
	userService := service.NewUserService(userRepo)
	groupService := service.NewGroupService(groupRepo, userRepo)
	expenseService := service.NewExpenseService(expenseRepo, groupRepo)
	balanceService := service.NewBalanceService(expenseRepo, groupRepo)
	authService := service.NewAuthService(userRepo)

	// 3. Init Handlers
	userHandler := handler.NewUserHandler(userService)
	groupHandler := handler.NewGroupHandler(groupService)
	expenseHandler := handler.NewExpenseHandler(expenseService)
	balanceHandler := handler.NewBalanceHandler(balanceService)
	authHandler := handler.NewAuthHandler(authService)

	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Ping endpoint for healthcheck
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "pong", "status": "ok"})
	})

	// Swagger documentation route
	r.Handle("/swagger/*", handler.SwaggerHandler())

	// Public Auth Routes
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Get("/google/login", authHandler.GoogleLogin)
		r.Get("/google/callback", authHandler.GoogleCallback)
	})

	// API Routes (Protected by JWT)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mymiddleware.JWTAuth)

		// Users
		r.Post("/users", userHandler.CreateUser)
		r.Get("/users/{id}", userHandler.GetUser)

		// Groups
		r.Post("/groups", groupHandler.CreateGroup)
		r.Get("/groups/{id}", groupHandler.GetGroup)
		r.Get("/groups/{id}/balances", balanceHandler.GetGroupBalances)

		// Expenses
		r.Post("/expenses", expenseHandler.AddExpense)
		r.Get("/groups/{groupId}/expenses", expenseHandler.GetGroupExpenses)
	})

	log.Println("Starting server on :8080...")
	err := http.ListenAndServe(":8080", r)
	if err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
