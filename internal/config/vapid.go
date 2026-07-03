package config

var (
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string
)

// InitVAPID reads the Web Push VAPID keypair. All optional at startup — push
// notifications are additive, so a missing keypair degrades to "no pushes
// sent" (see push_service.go) rather than crashing the whole server.
func InitVAPID() {
	VAPIDPublicKey = getEnv("VAPID_PUBLIC_KEY", "")
	VAPIDPrivateKey = getEnv("VAPID_PRIVATE_KEY", "")
	VAPIDSubject = getEnv("VAPID_SUBJECT", "")
}
