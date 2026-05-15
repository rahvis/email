package outbound

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func GenerateMessageID(fromEmail string) string {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		randomBytes = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	randomID := hex.EncodeToString(randomBytes)
	timestampMillis := time.Now().UnixMilli()

	domainPart := "billionmail"
	if parts := strings.SplitN(strings.TrimSpace(fromEmail), "@", 2); len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		domainPart = strings.TrimSpace(parts[1])
	}

	return fmt.Sprintf("<%d.%s@%s>", timestampMillis, randomID, domainPart)
}
