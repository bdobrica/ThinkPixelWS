package shared

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"
	"time"

	clockport "github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
)

// UUIDv7 is a time-ordered RFC 9562 UUIDv7 identifier.
type UUIDv7 [16]byte

// NewUUIDv7 uses an injectable clock and cryptographically secure randomness.
func NewUUIDv7(clock clockport.Clock) (UUIDv7, error) {
	return NewUUIDv7From(clock, rand.Reader)
}

func NewUUIDv7From(clock clockport.Clock, random io.Reader) (UUIDv7, error) {
	var id UUIDv7
	if clock == nil || random == nil {
		return id, NewError(CodeInvalidArgument, "clock and random source are required")
	}
	millis := clock.Now().UnixMilli()
	if millis < 0 || millis > 1<<48-1 {
		return id, NewError(CodeInvalidArgument, "clock is outside UUIDv7 range")
	}
	if _, err := io.ReadFull(random, id[6:]); err != nil {
		return id, WrapError(CodeInternal, "generate UUIDv7 randomness", err)
	}
	for i := 5; i >= 0; i-- {
		id[i] = byte(millis)
		millis >>= 8
	}
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}

func ParseUUIDv7(value string) (UUIDv7, error) {
	var id UUIDv7
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return id, NewError(CodeInvalidArgument, "identifier must be a canonical UUIDv7")
	}
	raw := strings.ReplaceAll(value, "-", "")
	decoded, err := hex.DecodeString(raw)
	if err != nil {
		return id, WrapError(CodeInvalidArgument, "identifier contains invalid hexadecimal", err)
	}
	copy(id[:], decoded)
	if id[6]>>4 != 7 || id[8]>>6 != 2 || id.String() != value {
		return UUIDv7{}, NewError(CodeInvalidArgument, "identifier must be a canonical UUIDv7")
	}
	return id, nil
}

func (id UUIDv7) String() string {
	var out [36]byte
	hex.Encode(out[0:8], id[0:4])
	out[8] = '-'
	hex.Encode(out[9:13], id[4:6])
	out[13] = '-'
	hex.Encode(out[14:18], id[6:8])
	out[18] = '-'
	hex.Encode(out[19:23], id[8:10])
	out[23] = '-'
	hex.Encode(out[24:36], id[10:16])
	return string(out[:])
}

func (id UUIDv7) Time() time.Time {
	millis := int64(0)
	for i := 0; i < 6; i++ {
		millis = millis<<8 | int64(id[i])
	}
	return time.UnixMilli(millis).UTC()
}

func (id UUIDv7) MarshalText() ([]byte, error) { return []byte(id.String()), nil }

func (id *UUIDv7) UnmarshalText(text []byte) error {
	parsed, err := ParseUUIDv7(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
