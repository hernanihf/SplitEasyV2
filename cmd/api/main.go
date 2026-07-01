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
	"time"

	"spliteasy/internal/config"
	"spliteasy/internal/handler"
	mymiddleware "spliteasy/internal/handler/middleware"
	"spliteasy/internal/repository"
	"spliteasy/internal/service"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present (local development only — ignored in production)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading config from environment variables")
	}

	// Initialize Database and Auth Configurations
	config.ConnectDB()
	config.InitAuth()
	config.InitAnthropic()

	// 1. Init Repositories
	userRepo := repository.NewUserRepository(config.DB)
	groupRepo := repository.NewGroupRepository(config.DB)
	expenseRepo := repository.NewExpenseRepository(config.DB)
	settlementRepo := repository.NewSettlementRepository(config.DB)

	// 2. Init Services
	userService := service.NewUserService(userRepo)
	groupService := service.NewGroupService(groupRepo, userRepo)
	expenseService := service.NewExpenseService(expenseRepo, groupRepo)
	balanceService := service.NewBalanceService(expenseRepo, groupRepo, settlementRepo)
	authService := service.NewAuthService(userRepo)
	receiptService := service.NewReceiptService(http.DefaultClient, config.AnthropicAPIKey, config.AnthropicModel)
	summaryService := service.NewSummaryService(groupRepo, expenseRepo, settlementRepo)

	// 3. Init Handlers
	userHandler := handler.NewUserHandler(userService)
	groupHandler := handler.NewGroupHandler(groupService)
	expenseHandler := handler.NewExpenseHandler(expenseService, groupService)
	balanceHandler := handler.NewBalanceHandler(balanceService, groupService)
	authHandler := handler.NewAuthHandler(authService)
	receiptHandler := handler.NewReceiptHandler(receiptService)
	summaryHandler := handler.NewSummaryHandler(summaryService)

	// Per-user rate limiter for the paid receipt-scan endpoint.
	scanLimiter := mymiddleware.NewScanRateLimiterFromEnv()

	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// CORS — the web/PWA frontend is served from a different origin than the
	// API. Origins are explicit (CORS_ALLOWED_ORIGINS env var, comma
	// separated) rather than a wildcard: a wildcard works today only because
	// auth is a Bearer JWT with no cookies involved, but it would silently
	// become a CSRF hole the moment any cookie-based credential is added.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

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

		// Home & activity
		r.Get("/home", summaryHandler.GetHome)
		r.Get("/activity", summaryHandler.GetActivity)

		// Users
		r.Post("/users", userHandler.CreateUser)
		r.Get("/users/me", userHandler.GetMe)
		r.Get("/users/{id}", userHandler.GetUser)

		// Groups
		r.Post("/groups", groupHandler.CreateGroup)
		r.Get("/groups", groupHandler.ListGroups)
		r.Post("/groups/join", groupHandler.JoinGroup)
		r.Get("/groups/{id}", groupHandler.GetGroup)
		r.Get("/groups/{id}/invite", groupHandler.GetInvite)
		r.Get("/groups/{id}/balances", balanceHandler.GetGroupBalances)
		r.Get("/groups/{id}/settlements", balanceHandler.ListSettlements)
		r.Post("/groups/{id}/settlements", balanceHandler.SettleDebt)

		// Expenses
		r.Post("/expenses", expenseHandler.AddExpense)
		r.Get("/groups/{groupId}/expenses", expenseHandler.GetGroupExpenses)

		// Receipts — rate limited per user (the scan is slow and billed by Anthropic)
		r.With(scanLimiter.Limit).Post("/receipts/scan", receiptHandler.ScanReceipt)
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Generous: the receipt scan calls Anthropic synchronously.
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Println("Starting server on :8080...")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
