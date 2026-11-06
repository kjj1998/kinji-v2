package ingest

import "regexp"

// otpKeyword matches wording banks use around one-time codes. Word boundaries keep short
// keywords like "otp" and "tac" from matching inside other words (e.g. "contact").
var otpKeyword = regexp.MustCompile(`(?i)\b(otp|tac|one[- ]time|verification code|security code|passcode|do not share)\b`)

// otpCode matches a standalone run of 4 to 8 digits, the usual shape of a one-time code.
var otpCode = regexp.MustCompile(`\b\d{4,8}\b`)

// looksLikeOTP reports whether body appears to contain a one-time password or security code.
// It needs both a keyword and a code, since transaction payloads always contain digits.
// It errs towards true: a dropped purchase shows up at statement reconciliation, but a
// stored OTP breaks the rule that codes are never stored.
func looksLikeOTP(body []byte) bool {
	return otpKeyword.Match(body) && otpCode.Match(body)
}
