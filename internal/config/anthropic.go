package config

var (
	AnthropicAPIKey string
	AnthropicModel  string
)

func InitAnthropic() {
	AnthropicAPIKey = getEnv("ANTHROPIC_API_KEY", "")
	AnthropicModel = getEnv("ANTHROPIC_MODEL", "claude-haiku-4-5")
}
