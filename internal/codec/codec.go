// Package codec is a minimal, self-contained subset of the original yaklang codec
// helpers used by the ported Java tooling. It is implemented purely on top of the
// Go standard library to keep javajive portable and dependency-light.
package codec

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// AnyToBytes converts an arbitrary value to its byte representation.
func AnyToBytes(i interface{}) []byte {
	switch v := i.(type) {
	case nil:
		return nil
	case []byte:
		return v
	case string:
		return []byte(v)
	case fmt.Stringer:
		return []byte(v.String())
	default:
		return []byte(fmt.Sprint(i))
	}
}

// AnyToString converts an arbitrary value to its string representation.
func AnyToString(i interface{}) string {
	switch v := i.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(i)
	}
}

// EncodeToHex hex-encodes the given value.
func EncodeToHex(i interface{}) string {
	return hex.EncodeToString(AnyToBytes(i))
}

// DecodeHex hex-decodes the given string.
func DecodeHex(i string) ([]byte, error) {
	return hex.DecodeString(i)
}

// DecodeBase64 decodes a standard base64 string.
func DecodeBase64(i string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(i)
}

// EncodeBase64 encodes the given value to a standard base64 string.
func EncodeBase64(i interface{}) string {
	return base64.StdEncoding.EncodeToString(AnyToBytes(i))
}

// Sha256 returns the lowercase hex SHA-256 digest of the given value.
func Sha256(i interface{}) string {
	sum := sha256.Sum256(AnyToBytes(i))
	return hex.EncodeToString(sum[:])
}
