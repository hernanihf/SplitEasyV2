package config

var (
	SupabaseURL            string
	SupabaseServiceRoleKey string
	SupabaseReceiptsBucket string
)

// InitSupabase reads Supabase Storage config. All three are optional at
// startup (getEnv, not mustGetEnv) — receipt image persistence is additive
// to the app's core function, so a missing/unset key degrades to "scans
// still work, images just aren't saved" (see storage_service.go /
// receipt_service.go) rather than crashing the whole server.
func InitSupabase() {
	SupabaseURL = getEnv("SUPABASE_URL", "")
	SupabaseServiceRoleKey = getEnv("SUPABASE_SERVICE_ROLE_KEY", "")
	SupabaseReceiptsBucket = getEnv("SUPABASE_RECEIPTS_BUCKET", "receipts")
}
