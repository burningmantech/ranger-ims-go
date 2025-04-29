package password

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	saltPasswordSep = ":"
)

func Verify(password, storedValue string) (isValid bool, err error) {
	salt, storedHash, found := strings.Cut(storedValue, saltPasswordSep)
	if !found {
		return false, fmt.Errorf("invalid hashed password")
	}
	return hash(password, salt) == storedHash, nil
}

func hash(password, salt string) string {
	hasher := sha1.New()
	hasher.Write([]byte(salt + password))
	return hex.EncodeToString(hasher.Sum(nil))
}

func NewSalted(password string) string {
	salt := rand.Text()
	return salt + ":" + hash(password, salt)
}
