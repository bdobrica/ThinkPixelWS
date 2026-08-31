package shared

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// BoundedString is validated UTF-8 text in NFC form without control
// characters. Limits are counted in Unicode code points, matching JSON Schema
// string length semantics.
type BoundedString string

func NewBoundedString(value string, minRunes, maxRunes int) (BoundedString, error) {
	if minRunes < 0 || maxRunes < minRunes {
		return "", NewError(CodeInvalidArgument, "invalid string bounds")
	}
	if !utf8.ValidString(value) {
		return "", NewError(CodeInvalidArgument, "string is not valid UTF-8")
	}
	if !norm.NFC.IsNormalString(value) {
		return "", NewError(CodeInvalidArgument, "string is not NFC-normalized")
	}
	length := utf8.RuneCountInString(value)
	if length < minRunes || length > maxRunes {
		return "", NewError(CodeInvalidArgument, fmt.Sprintf("string length must be between %d and %d", minRunes, maxRunes))
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", NewError(CodeInvalidArgument, "string contains a control character")
		}
	}
	return BoundedString(value), nil
}

func (s BoundedString) String() string { return string(s) }
