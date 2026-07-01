package handler

import "fmt"

// Upper bounds for free-text request fields. Without a limit, a client can
// send an arbitrarily large string (Postgres text columns accept up to ~1GB)
// that gets stored as-is — cheap storage/bandwidth abuse with no legitimate
// use case anywhere near these sizes.
const (
	maxNameLen        = 100
	maxEmojiLen       = 32 // compound emoji (skin tone/ZWJ sequences) can run several bytes
	maxDescriptionLen = 500
	maxTokenLen       = 256
	maxCommentLen     = 1000
)

func validateMaxLen(field, value string, max int) error {
	if len(value) > max {
		return fmt.Errorf("%s must be at most %d characters", field, max)
	}
	return nil
}
