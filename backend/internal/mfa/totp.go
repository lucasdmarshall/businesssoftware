// Package mfa implements RFC 6238 time-based one-time passwords (TOTP) for the
// privileged-user MFA foundation. It is self-contained (standard library only)
// so it can be unit tested against the RFC test vectors and bundled offline.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Period is the TOTP time step in seconds (30 is the interoperable default).
const Period = 30

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random base32 secret suitable for authenticator
// apps.
func GenerateSecret() (string, error) {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return encoding.EncodeToString(buf), nil
}

// code computes the 6-digit HOTP value for a counter.
func code(secret string, counter uint64) (string, error) {
	key, err := encoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}
	var message [8]byte
	binary.BigEndian.PutUint64(message[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", value%1_000_000), nil
}

// Validate reports whether input matches the TOTP for secret at time `at`,
// allowing a one-step window on each side to tolerate clock skew.
func Validate(secret, input string, at time.Time) bool {
	input = strings.TrimSpace(input)
	if len(input) != 6 {
		return false
	}
	counter := uint64(at.Unix()) / Period
	for _, delta := range []int64{0, -1, 1} {
		candidate, err := code(secret, uint64(int64(counter)+delta))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(candidate), []byte(input)) {
			return true
		}
	}
	return false
}

// OTPAuthURI returns the otpauth:// URI an authenticator app scans to enroll.
func OTPAuthURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("period", fmt.Sprintf("%d", Period))
	query.Set("digits", "6")
	query.Set("algorithm", "SHA1")
	return "otpauth://totp/" + label + "?" + query.Encode()
}
