// Package ids generates opaque identifiers.
package ids

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// New returns a 32-char hex id.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// Code returns a short human-typeable code (invite codes), e.g. "K7Q2-M9XA".
func Code() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	s := strings.TrimRight(base32.StdEncoding.EncodeToString(b[:]), "=")
	return s[:4] + "-" + s[4:8]
}

// Token returns a 43-char URL-safe secret.
func Token() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b[:]), "=")
}
