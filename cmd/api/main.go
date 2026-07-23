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
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
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

// maxJSONBodyBytes caps every JSON request body except the receipt scan
// (which needs room for an image and sets its own, larger limit). Far more
// than any legitimate payload here — the largest realistic body is an
// expense with a long description and a page of splits/items — while still
// nowhere near enough for a memory-exhaustion attempt to be worthwhile.
const maxJSONBodyBytes = 1 << 20 // 1MB

// maxImportJSONBodyBytes covers the CSV-import confirm step, which echoes
// back every parsed row (one per historical expense) instead of a single
// record — a multi-year, many-member history can outgrow maxJSONBodyBytes.
const maxImportJSONBodyBytes = 8 << 20 // 8MB

func main() {
	// JSON structured logging: production log aggregators need machine-parsable
	// output to filter by level/field, which the stdlib "log" package can't do.
	// Wrapped in RedactingHandler as a safety net against a future call site
	// accidentally logging a token/header/secret — no call site does today.
	slog.SetDefault(slog.New(config.NewRedactingHandler(slog.NewJSONHandler(os.Stdout, nil))))

	// Load .env file if present (local development only — ignored in production)
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, reading config from environment variables")
	}

	// Initialize Database and Auth Configurations
	db, err := config.ConnectDB()
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	config.InitAuth()
	config.InitAnthropic()
	config.InitSupabase()
	config.InitVAPID()

	// Refresh tokens are stored in Redis when available (REDIS_URL), so a
	// stolen/rotated token can be revoked from any instance and survives
	// restarts; otherwise they fall back to in-memory (local dev only — same
	// trade-off as the scan rate limiter's in-memory fallback).
	var refreshStore service.RefreshTokenStore
	if rdb, ok := config.NewRedisClientFromEnv(); ok {
		refreshStore = service.NewRedisRefreshTokenStore(rdb)
	} else {
		slog.Warn("REDIS_URL not set, refresh tokens are in-memory and will reset on every deploy")
		refreshStore = service.NewInMemoryRefreshTokenStore()
	}

	// 1. Init Repositories
	userRepo := repository.NewUserRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	expenseRepo := repository.NewExpenseRepository(db)
	settlementRepo := repository.NewSettlementRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	pushSubRepo := repository.NewPushSubscriptionRepository(db)
	jobRunRepo := repository.NewJobRunRepository(db)
	auditLogRepo := repository.NewAuditLogRepository(db)

	// 2. Init Services
	userService := service.NewUserService(userRepo)
	expenseService := service.NewExpenseService(expenseRepo, groupRepo)
	balanceService := service.NewBalanceService(expenseRepo, groupRepo, settlementRepo)
	authService := service.NewAuthService(userRepo, refreshStore)
	storageService := service.NewStorageService(http.DefaultClient, config.SupabaseURL, config.SupabaseServiceRoleKey, config.SupabaseReceiptsBucket)
	if storageService == nil {
		slog.Warn("SUPABASE_URL/SUPABASE_SERVICE_ROLE_KEY not set, receipt images will not be persisted")
	}
	groupService := service.NewGroupService(groupRepo, userRepo, storageService)
	receiptService := service.NewReceiptService(http.DefaultClient, config.AnthropicAPIKey, config.AnthropicModel, storageService)
	summaryService := service.NewSummaryService(groupRepo, expenseRepo, settlementRepo, commentRepo, userRepo)
	commentService := service.NewCommentService(commentRepo)
	pushService := service.NewPushService(groupRepo, userRepo, pushSubRepo, http.DefaultClient, config.VAPIDPublicKey, config.VAPIDPrivateKey, config.VAPIDSubject)
	if config.VAPIDPublicKey == "" || config.VAPIDPrivateKey == "" {
		slog.Warn("VAPID_PUBLIC_KEY/VAPID_PRIVATE_KEY not set, push notifications will not be sent")
	}
	purgeService := service.NewPurgeService(expenseRepo, jobRunRepo, storageService)
	auditService := service.NewAuditService(auditLogRepo)
	importService := service.NewImportService(groupRepo, expenseRepo, settlementRepo)

	// 3. Init Handlers
	userHandler := handler.NewUserHandler(userService, pushService)
	groupHandler := handler.NewGroupHandler(groupService, auditService)
	expenseHandler := handler.NewExpenseHandler(expenseService, groupService, storageService, pushService, auditService)
	importHandler := handler.NewImportHandler(importService, groupService, auditService)
	balanceHandler := handler.NewBalanceHandler(balanceService, groupService, pushService, auditService)
	authHandler := handler.NewAuthHandler(authService)
	receiptHandler := handler.NewReceiptHandler(receiptService)
	summaryHandler := handler.NewSummaryHandler(summaryService)
	commentHandler := handler.NewCommentHandler(commentService, expenseService, balanceService, groupService, pushService)
	pushHandler := handler.NewPushHandler(pushService)

	// Per-user rate limiter for the paid receipt-scan endpoint.
	scanLimiter := mymiddleware.NewScanRateLimiterFromEnv()

	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(mymiddleware.RequestLogger)
	r.Use(mymiddleware.Recoverer)

	// CORS — the web/PWA frontend is served from a different origin than the
	// API. Origins are explicit (CORS_ALLOWED_ORIGINS env var, comma
	// separated) rather than a wildcard: a wildcard works today only because
	// auth is a Bearer JWT with no cookies involved, but it would silently
	// become a CSRF hole the moment any cookie-based credential is added.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   config.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Ping endpoint for healthcheck
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "pong", "status": "ok"}); err != nil {
			slog.Error("failed to encode /ping response", "error", err)
		}
	})

	// Swagger documentation route
	r.Handle("/swagger/*", handler.SwaggerHandler())

	// Public Auth Routes
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Use(mymiddleware.MaxBytes(maxJSONBodyBytes))
		r.Get("/google/login", authHandler.GoogleLogin)
		r.Get("/google/callback", authHandler.GoogleCallback)
		r.Post("/refresh", authHandler.Refresh)
		r.Post("/logout", authHandler.Logout)
	})

	// Public group preview — deliberately a standalone route (not nested
	// under a "/api/v1/groups" sub-router) so it can't shadow the protected
	// /groups and /groups/{id} routes chi registers below; a single r.Get
	// here coexists fine with those, a second Route() on the same prefix
	// would not. No JWTAuth: the whole point is to preview before signing
	// in, and the invite token is all the credential PreviewGroup needs.
	r.With(mymiddleware.MaxBytes(maxJSONBodyBytes)).Get("/api/v1/groups/preview", groupHandler.PreviewGroup)

	// API Routes (Protected by JWT)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(mymiddleware.JWTAuth)

		// Everything except the receipt scan gets the small JSON body cap;
		// scan needs its own, larger one for image uploads (set inside the
		// handler), and nesting MaxBytesReader only ever tightens it further.
		r.Group(func(r chi.Router) {
			r.Use(mymiddleware.MaxBytes(maxJSONBodyBytes))

			// Home & activity
			r.Get("/home", summaryHandler.GetHome)
			r.Get("/activity", summaryHandler.GetActivity)
			r.Get("/activity/unread-count", summaryHandler.GetUnreadActivityCount)
			r.Post("/activity/seen", summaryHandler.MarkActivitySeen)

			// Users
			r.Get("/users/me", userHandler.GetMe)
			r.Patch("/users/me/push-preference", userHandler.SetPushPreference)

			// Push subscriptions
			r.Post("/push/subscribe", pushHandler.Subscribe)
			r.Delete("/push/subscribe", pushHandler.Unsubscribe)

			// Groups
			r.Post("/groups", groupHandler.CreateGroup)
			r.Get("/groups", groupHandler.ListGroups)
			r.Post("/groups/join", groupHandler.JoinGroup)
			r.Get("/groups/{id}", groupHandler.GetGroup)
			r.Patch("/groups/{id}", groupHandler.UpdateGroup)
			r.Delete("/groups/{id}", groupHandler.DeleteGroup)
			r.Get("/groups/{id}/invite", groupHandler.GetInvite)
			r.Get("/groups/{id}/export.csv", importHandler.ExportGroupCSV)
			r.Get("/groups/{id}/balances", balanceHandler.GetGroupBalances)
			r.Get("/groups/{id}/settlements", balanceHandler.ListSettlements)
			r.Post("/groups/{id}/settlements", balanceHandler.SettleDebt)
			r.Get("/settlements/{id}", balanceHandler.GetSettlement)
			r.Delete("/settlements/{id}", balanceHandler.DeleteSettlement)
			r.Post("/settlements/{id}/comments", commentHandler.AddSettlementComment)
			r.Get("/settlements/{id}/comments", commentHandler.ListSettlementComments)

			// Expenses
			r.Post("/expenses", expenseHandler.AddExpense)
			r.Get("/expenses/{id}", expenseHandler.GetExpense)
			r.Put("/expenses/{id}", expenseHandler.UpdateExpense)
			r.Delete("/expenses/{id}", expenseHandler.DeleteExpense)
			r.Get("/groups/{groupId}/expenses", expenseHandler.GetGroupExpenses)
			r.Post("/expenses/{id}/comments", commentHandler.AddExpenseComment)
			r.Get("/expenses/{id}/comments", commentHandler.ListExpenseComments)

			// Comments (delete is flat, like settlements — the author check
			// is self-contained and doesn't need a separate parent id).
			r.Delete("/comments/{id}", commentHandler.DeleteComment)
		})

		// Receipts — rate limited per user (the scan is slow and billed by Anthropic)
		r.With(scanLimiter.Limit).Post("/receipts/scan", receiptHandler.ScanReceipt)

		// CSV import — the file upload and the confirm step's row-by-row JSON
		// body can both run well past the small JSON cap for a group with a
		// long history, so each gets its own larger limit (set inside
		// PreviewImport for the multipart upload; here for the JSON one).
		r.Post("/groups/{id}/import/preview", importHandler.PreviewImport)
		r.With(mymiddleware.MaxBytes(maxImportJSONBodyBytes)).Post("/groups/{id}/import", importHandler.ConfirmImport)
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

	go runPeriodicPurge(purgeService)

	slog.Info("starting server", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server failed to start", "error", err)
		os.Exit(1)
	}
}

// purgeTickInterval is how often each instance *attempts* the purge, not how
// often it actually happens — JobRunRepository.TryClaim (via purgeService)
// throttles the real work to roughly once a day across the whole fleet.
// Ticking more often than that just gives an instance more chances to catch
// the claim if it's asleep (e.g. Render free tier) when the window opens.
const purgeTickInterval = 2 * time.Hour

// runPeriodicPurge runs the expense-purge job once immediately, then on a
// ticker, for as long as the process lives. Safe to run on every
// instance — see purgeJobMinInterval in purge_service.go.
func runPeriodicPurge(purgeService service.PurgeService) {
	ctx := context.Background()
	if err := purgeService.PurgeOldExpenses(ctx); err != nil {
		slog.Error("purge job failed", "error", err)
	}

	ticker := time.NewTicker(purgeTickInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := purgeService.PurgeOldExpenses(ctx); err != nil {
			slog.Error("purge job failed", "error", err)
		}
	}
}
