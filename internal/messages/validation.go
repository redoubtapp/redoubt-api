package messages

import (
	"strings"
	"unicode/utf8"

	apperrors "github.com/redoubtapp/redoubt-api/internal/errors"
)

const (
	// MaxMessageLength is the maximum allowed message length in characters.
	MaxMessageLength = 2000
	// MinMessageLength is the minimum allowed message length.
	MinMessageLength = 1
	// MaxCodeBlockLength is the maximum allowed code block length in characters.
	MaxCodeBlockLength = 1500
)

// ValidateContent validates message content.
func ValidateContent(content string) error {
	content = strings.TrimSpace(content)

	if len(content) == 0 {
		return apperrors.ErrMessageEmpty
	}

	if utf8.RuneCountInString(content) > MaxMessageLength {
		return apperrors.ErrMessageTooLong
	}

	if hasOversizedCodeBlock(content) {
		return apperrors.ErrCodeBlockTooLong
	}

	return nil
}

// hasOversizedCodeBlock checks for code blocks exceeding the maximum length.
func hasOversizedCodeBlock(content string) bool {
	inBlock := false
	blockStart := 0

	for i := 0; i < len(content)-2; i++ {
		if content[i:i+3] == "```" {
			if !inBlock {
				inBlock = true
				blockStart = i + 3
				// Skip to end of opening line (language identifier)
				for blockStart < len(content) && content[blockStart] != '\n' {
					blockStart++
				}
			} else {
				blockLen := i - blockStart
				if blockLen > MaxCodeBlockLength {
					return true
				}
				inBlock = false
			}
		}
	}

	return false
}
