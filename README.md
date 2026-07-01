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
    *   JWT-based session management for protected routes.
*   **Groups:**
    *   Create expense-sharing groups.
    *   Add and manage group members.
*   **Expense Sharing:**
    *   Log expenses indicating payer, total amount, and split distribution.
    *   Support for multiple splitting methods:
        *   **Equal parts:** Split the expense evenly among selected members.
        *   **Percentages:** Split by specific percentage targets (e.g., 50% User A, 50% User B).
        *   **Fixed amounts:** Allocate exact amounts to each member (e.g., $100 User A, $200 User B).
        *   **Variable quantities / shares:** Split by weights or unit counts (e.g., 2 units of bread User A, 4 units of bread User B).
*   **Settlements & Balances:**
    *   Real-time balance calculations resolving "who owes how much to whom" efficiently.
    *   Settle up balances (marking debts as paid).
*   **AI-Powered Ticket Scanner:**
    *   Scan or upload a photo of a receipt; Claude (Anthropic) vision extracts the merchant name, date, total amount, and line items, ready to prefill a new expense.

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

# Frontend URL the user is redirected to after Google login, with the JWT in the URL fragment (#token=...)
FRONTEND_REDIRECT_URL=http://localhost:8081/auth/callback

# Comma-separated list of origins allowed by CORS. Production must set this to
# the frontend's real domain (e.g. https://spliteasy-app.onrender.com) — no wildcard.
CORS_ALLOWED_ORIGINS=http://localhost:8081

# Anthropic (Claude) — used to parse photographed receipts. Get one at: https://console.anthropic.com
ANTHROPIC_API_KEY=
ANTHROPIC_MODEL=claude-haiku-4-5
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

### Public Routes

*   `GET /ping` - Health check (returns database/application status).
*   `GET /api/v1/auth/google/login` - Initiates Google OAuth2 login flow.
*   `GET /api/v1/auth/google/callback` - Handles the Google OAuth2 callback, then redirects to `FRONTEND_REDIRECT_URL#token=<JWT>`.

### Protected Routes (Requires Header: `Authorization: Bearer <JWT_TOKEN>`)

#### Users
*   `GET /api/v1/users/{id}` - Retrieve user details by ID.

#### Groups
*   `POST /api/v1/groups` - Create a new expense sharing group. The authenticated user becomes its creator and first member.
*   `GET /api/v1/groups` - List the groups the authenticated user belongs to.
*   `GET /api/v1/groups/{id}` - Get group details (including members).
*   `GET /api/v1/groups/{id}/balances` - Get outstanding balances and debts (who owes who) for a specific group, net of recorded settlements.
*   `POST /api/v1/groups/{id}/settlements` - Record a payment between two group members, reducing their outstanding balance ("settle up").

#### Expenses
*   `POST /api/v1/expenses` - Create a new expense and split it among group members. `split_method` can be:
    *   `equal` (default) - splits evenly among all group members, or the given `splits[].user_id` subset.
    *   `percentage` - splits according to `splits[].value` (0-100), which must add up to 100.
    *   `fixed` - splits according to `splits[].value` exact amounts, which must add up to the total.
    *   `shares` - splits proportionally to `splits[].value` relative weights/units.
*   `GET /api/v1/groups/{groupId}/expenses` - List all expenses logged in a group.

#### Receipts
*   `POST /api/v1/receipts/scan` - Upload a receipt photo (`multipart/form-data`, field `image`); returns `{merchant_name, date, total_amount, items[]}` extracted by Claude vision, for prefilling a new expense. Requires `ANTHROPIC_API_KEY` to be configured.
