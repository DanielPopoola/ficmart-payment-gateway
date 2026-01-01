package services

import (
	"crypto/sha256"
	"fmt"
)

func ComputeHash(v interface{}) string {
	data := fmt.Sprintf("%+v", v)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
