# SplitEasy

SplitEasy is a robust, performant backend API written in Go, designed to manage shared expenses within groups (similar to Splitwise). It is built using clean architecture principles, emphasizing strict separation of concerns, SOLID design patterns, and high testability.

## 🌐 Deployment

| Component | URL | Hosting |
|---|---|---|
| Backend API | https://spliteasyv2.onrender.com | Render (Docker web service) |
| Frontend (PWA) | https://spliteasy-app.onrender.com | Render (static site, Expo web export) |

The frontend is an Expo + React Native Web app exported to a static site (`npx expo export -p web` → `dist/`). It reads the API base URL from the build-time env var `EXPO_PUBLIC_API_URL`. The API enables CORS so the browser/PWA can call it cross-origin.

## 🚀 Key Features

*   **User Management & Security:**
    *   Authentication via Google OAuth 2.0.
    *   JWT access tokens plus rotating refresh tokens (`/auth/refresh`, `/auth/logout`).
*   **Groups:**
    *   Create, rename, re-icon, and delete expense-sharing groups.
    *   Invite-link based joining: a shareable token lets anyone preview a group (name, icon, creator, member count) before signing in, then join with one call once authenticated.
    *   Export a group's full history as a Splitwise-compatible CSV, or import one to seed a new group's expenses.
*   **Expense Sharing:**
    *   Log expenses indicating payer, total amount, and split distribution.
    *   Edit or soft-delete an expense (only the payer or a split participant may); a deleted expense stays viewable read-only to everyone in the group instead of vanishing outright, and is permanently purged (along with its receipt image) after a retention window by a background job.
    *   Itemized splits: break an expense into line items, each assigned to whichever subset of members shared it — plus free-form "adjustment" lines (negative or positive amounts) to reflect a discount, tip, or fee that isn't tied to a specific item.
    *   Support for multiple splitting methods:
        *   **Equal parts:** Split the expense evenly among selected members.
        *   **Percentages:** Split by specific percentage targets (e.g., 50% User A, 50% User B).
        *   **Fixed amounts:** Allocate exact amounts to each member (e.g., $100 User A, $200 User B).
        *   **Variable quantities / shares:** Split by weights or unit counts (e.g., 2 units of bread User A, 4 units of bread User B).
    *   Comment threads on any expense or settlement; authors can delete their own comments.
*   **Settlements & Balances:**
    *   Real-time balance calculations resolving "who owes how much to whom" efficiently.
    *   Settle up balances (marking debts as paid), list or fetch a group's settlements, delete one, and comment on it.
*   **Activity Feed & Home Summary:**
    *   A cross-group activity feed combining expenses, settlements, and comments, newest first.
    *   Home dashboard: overall balance per currency plus a per-group summary.
    *   Unread-activity badge (`/activity/unread-count`), cleared by `/activity/seen` once the user has viewed the feed.
*   **Push Notifications:**
    *   Web Push (VAPID) subscriptions per device, with a per-user on/off preference (`PATCH /users/me/push-preference`). Degrades to a no-op with a startup warning if `VAPID_PUBLIC_KEY`/`VAPID_PRIVATE_KEY` aren't configured.
*   **AI-Powered Ticket Scanner:**
    *   Scan or upload a photo (or PDF) of a receipt; Claude (Anthropic) vision extracts the merchant name, date, total amount, and line items, ready to prefill a new expense. Rate-limited per user, and backed by Redis when `REDIS_URL` is set (falls back to an in-memory limiter otherwise).
*   **Receipt Image Storage:**
    *   Uploaded receipt images are persisted to Supabase Storage when `SUPABASE_URL`/`SUPABASE_SERVICE_ROLE_KEY` are configured; otherwise the app keeps working but doesn't retain the images.

---

## 🏗️ Architecture & Project Structure

The project conforms to the **Standard Go Project Layout** and follows **Clean Architecture / Ports & Adapters** to decouple business logic from framework dependencies.

```text
SplitEasy/
├── cmd/
│   └── api/                # Main entry point for the REST API
│       ├── main.go         # App wiring (Dependency Injection) and server initialization
│       └── main_test.go    # API integration tests
├── internal/
│   ├── config/             # DB connection, OAuth configuration, environment variables
│   ├── domain/             # Core entity models (User, Group, Expense, Split, Balance)
│   ├── handler/            # Delivery layer (HTTP handlers, router setup)
│   │   └── middleware/     # Custom HTTP middlewares (JWT auth, logging, etc.)
│   ├── repository/         # Data layer (GORM / Postgres implementations)
│   └── service/            # Business Logic layer (calculators, rule engines)
├── Dockerfile              # Multi-stage production Docker build configuration
├── go.mod                  # Go module definition
└── requerimientos.md       # Product specifications and requirements
```

### Key Technical Guidelines (Clean Code Principles)
*   **Separation of Concerns:** Business logic (services) is blind to how data is presented (handlers) or stored (repositories).
*   **Dependency Injection:** Dependencies are configured and injected in `main.go`.
*   **Immutability:** Domain data is treated as immutable by default to prevent unexpected side effects.
*   **Error Handling:** Errors are propagated explicitly up the stack to handlers for standard API error responses.

---

## 🛠️ Getting Started

### Prerequisites

*   [Go](https://go.dev/) (v1.26.3 or higher recommended)
*   [PostgreSQL](https://www.postgresql.org/) (running instance)
*   [Docker](https://www.docker.com/) (optional, for containerized deployment)

### Environment Variables

Before running the application, configure your environment variables. You can set them in your system environment or create a `.env` file (if supported by your runner):

```bash
DB_HOST=localhost
DB_USER=postgres
DB_PASSWORD=yourpassword
DB_NAME=spliteasy
DB_PORT=5432
DB_SSLMODE=disable

JWT_SECRET=your_jwt_secret_key
GOOGLE_CLIENT_ID=your_google_client_id
GOOGLE_CLIENT_SECRET=your_google_client_secret
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback

# Frontend URL the user is redirected to after Google login, with the access
# and refresh tokens in the URL fragment (#access_token=...&refresh_token=...)
FRONTEND_REDIRECT_URL=http://localhost:8081/auth/callback

# Comma-separated list of origins allowed by CORS. Production must set this to
# the frontend's real domain (e.g. https://spliteasy-app.onrender.com) — no wildcard.
CORS_ALLOWED_ORIGINS=http://localhost:8081

# Anthropic (Claude) — used to parse photographed receipts. Get one at: https://console.anthropic.com
ANTHROPIC_API_KEY=
ANTHROPIC_MODEL=claude-haiku-4-5

# Optional — Redis, shared by the receipt-scan rate limiter and refresh
# tokens. Without it, the app still runs (an in-memory limiter takes over).
REDIS_URL=

# Optional — Supabase Storage, where uploaded receipt images are persisted.
# Without it, receipt scanning still works but images aren't kept.
SUPABASE_URL=
SUPABASE_SERVICE_ROLE_KEY=
SUPABASE_RECEIPTS_BUCKET=receipts

# Optional — VAPID keys for Web Push notifications. Without them, push
# subscriptions are accepted but nothing is ever sent. Generate a pair with
# `npx web-push generate-vapid-keys`.
VAPID_PUBLIC_KEY=
VAPID_PRIVATE_KEY=
VAPID_SUBJECT=mailto:you@example.com
```

### Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/your-username/SplitEasy.git
    cd SplitEasy
    ```

2.  **Download dependencies:**
    ```bash
    go mod download
    ```

3.  **Run the application locally:**
    ```bash
    go run cmd/api/main.go
    ```
    The server will start on port `8080` (e.g., `http://localhost:8080`).

4.  **Run tests:**
    ```bash
    go test ./...
    ```

### Testing

*   **Automated tests** (business logic: split methods, balances, settlements, AI response parsing):
    ```bash
    go test ./internal/service/...
    ```
*   **Manual end-to-end tests** — reproducible `curl` recipes for every endpoint (named test cases, expected responses, and the real AI receipt-scan output) are documented in [docs/MANUAL_TESTING.md](docs/MANUAL_TESTING.md).

### Running with Docker

You can package and run the application inside a lightweight container:

```bash
# Build the Docker image
docker build -t spliteasy .

# Run the container
docker run -p 8080:8080 --env-file .env spliteasy
```

---

## 🔌 API Endpoints Reference

Full interactive docs (request/response schemas) are also served at `/swagger/index.html`.

### Public Routes

*   `GET /ping` - Health check (returns database/application status).
*   `GET /api/v1/auth/google/login` - Initiates Google OAuth2 login flow.
*   `GET /api/v1/auth/google/callback` - Handles the Google OAuth2 callback, then redirects to `FRONTEND_REDIRECT_URL#access_token=<JWT>&refresh_token=<TOKEN>`.
*   `POST /api/v1/auth/refresh` - Exchanges a valid, unused refresh token for a new access token and a new (rotated) refresh token.
*   `POST /api/v1/auth/logout` - Revokes a refresh token so it can no longer be exchanged.
*   `GET /api/v1/groups/preview?token=` - Preview a group (name, icon, creator, member count) by its invite token, before signing in.

### Protected Routes (Requires Header: `Authorization: Bearer <JWT_TOKEN>`)

#### Home & Activity
*   `GET /api/v1/home` - Dashboard summary: overall balance per currency, plus a per-group summary.
*   `GET /api/v1/activity` - Cross-group activity feed (expenses, settlements, and comments), newest first.
*   `GET /api/v1/activity/unread-count` - Count of activity events since the user last viewed the feed.
*   `POST /api/v1/activity/seen` - Marks the feed as viewed as of now, clearing the unread badge.

#### Users
*   `GET /api/v1/users/me` - Retrieve the authenticated user's profile.
*   `PATCH /api/v1/users/me/push-preference` - Turn push notifications on/off for the account.

#### Push
*   `POST /api/v1/push/subscribe` - Register this device's Web Push subscription.
*   `DELETE /api/v1/push/subscribe` - Remove this device's Web Push subscription.

#### Groups
*   `POST /api/v1/groups` - Create a new expense sharing group. The authenticated user becomes its creator and first member.
*   `GET /api/v1/groups` - List the groups the authenticated user belongs to.
*   `GET /api/v1/groups/{id}` - Get group details (including members).
*   `PATCH /api/v1/groups/{id}` - Update a group's name and/or icon.
*   `DELETE /api/v1/groups/{id}` - Delete a group.
*   `GET /api/v1/groups/{id}/invite` - Get (or create) the group's invite link/token.
*   `POST /api/v1/groups/join` - Join a group via an invite token.
*   `GET /api/v1/groups/{id}/export.csv` - Export the group's expense history as a Splitwise-compatible CSV.
*   `POST /api/v1/groups/{id}/import/preview` - Upload a Splitwise-style expense CSV (`multipart/form-data`) and get back a parsed, editable preview.
*   `POST /api/v1/groups/{id}/import` - Confirm a previously-previewed import, creating the expenses.
*   `GET /api/v1/groups/{id}/balances` - Get outstanding balances and debts (who owes who) for a specific group, net of recorded settlements.

#### Settlements
*   `GET /api/v1/groups/{id}/settlements` - List a group's settlements.
*   `POST /api/v1/groups/{id}/settlements` - Record a payment between two group members, reducing their outstanding balance ("settle up").
*   `GET /api/v1/settlements/{id}` - Get a single settlement.
*   `DELETE /api/v1/settlements/{id}` - Delete a settlement.
*   `POST /api/v1/settlements/{id}/comments` - Comment on a settlement.
*   `GET /api/v1/settlements/{id}/comments` - List a settlement's comments.

#### Expenses
*   `POST /api/v1/expenses` - Create a new expense and split it among group members. `split_method` can be:
    *   `equal` (default) - splits evenly among all group members, or the given `splits[].user_id` subset.
    *   `percentage` - splits according to `splits[].value` (0-100), which must add up to 100.
    *   `fixed` - splits according to `splits[].value` exact amounts, which must add up to the total.
    *   `shares` - splits proportionally to `splits[].value` relative weights/units.

    Optionally pass `items[]` (`{description, amount, user_ids}`) to itemize the expense — each item's `amount` can be negative to represent a discount/tip/fee. Items are persisted for display only; they don't drive the balances (the `splits`/`split_method` you send do).
*   `GET /api/v1/expenses/{id}` - Get a single expense. Any member of its group may view it (including a soft-deleted one, read-only) — unlike edit/delete, this isn't limited to the payer or split participants.
*   `PUT /api/v1/expenses/{id}` - Replace an expense's payer, description, amount, split, and items. Only the current payer or a current split participant may edit it.
*   `DELETE /api/v1/expenses/{id}` - Soft-delete an expense (only the payer or a split participant may). It stays visible read-only until a background job permanently purges it after the retention window.
*   `GET /api/v1/groups/{groupId}/expenses` - List all expenses logged in a group.
*   `POST /api/v1/expenses/{id}/comments` - Comment on an expense.
*   `GET /api/v1/expenses/{id}/comments` - List an expense's comments.

#### Comments
*   `DELETE /api/v1/comments/{id}` - Delete a comment (only its author may).

#### Receipts
*   `POST /api/v1/receipts/scan` - Upload a receipt image or PDF (`multipart/form-data`, field `image`); returns `{merchant_name, date, total_amount, items[]}` extracted by Claude vision, for prefilling a new expense. Requires `ANTHROPIC_API_KEY`; rate-limited per user.
