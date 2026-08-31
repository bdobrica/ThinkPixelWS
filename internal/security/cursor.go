package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/bdobrica/ThinkPixelWS/internal/domain/shared"
	clockport "github.com/bdobrica/ThinkPixelWS/internal/ports/clock"
)

const (
	cursorVersion    = 1
	maxCursorLength  = 2048
	maxCursorPayload = 1024
	maxCursorScope   = 128
)

var errInvalidCursor = errors.New("invalid cursor")

type cursorEnvelope struct {
	Version   int             `json:"v"`
	Scope     string          `json:"s"`
	ExpiresAt int64           `json:"e"`
	Payload   json.RawMessage `json:"p"`
}

// CursorCodec authenticates opaque pagination state with HMAC-SHA-256. Scope
// binds a cursor to one list operation and its tenant/principal/filter context.
type CursorCodec struct {
	key   []byte
	clock clockport.Clock
}

func NewCursorCodec(key []byte, clock clockport.Clock) (*CursorCodec, error) {
	if len(key) < 32 || clock == nil {
		return nil, shared.NewError(shared.CodeInvalidArgument, "cursor key must be at least 32 bytes and clock is required")
	}
	return &CursorCodec{key: append([]byte(nil), key...), clock: clock}, nil
}

func (c *CursorCodec) Encode(scope string, payload any, expiresAt time.Time) (string, error) {
	if scope == "" || len(scope) > maxCursorScope || !expiresAt.After(c.clock.Now()) {
		return "", shared.NewError(shared.CodeInvalidArgument, "cursor scope or expiry is invalid")
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > maxCursorPayload {
		return "", shared.WrapError(shared.CodeInvalidArgument, "cursor payload is invalid or too large", err)
	}
	envelope := cursorEnvelope{Version: cursorVersion, Scope: scope, ExpiresAt: expiresAt.UTC().Unix(), Payload: raw}
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", shared.WrapError(shared.CodeInternal, "encode cursor", err)
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(body)
	token := base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(token) > maxCursorLength {
		return "", shared.NewError(shared.CodeInvalidArgument, "cursor exceeds maximum length")
	}
	return token, nil
}

func (c *CursorCodec) Decode(token, expectedScope string, destination any) error {
	if token == "" || len(token) > maxCursorLength || expectedScope == "" {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	dot := -1
	for i := range token {
		if token[i] == '.' {
			if dot != -1 {
				return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
			}
			dot = i
		}
	}
	if dot <= 0 || dot == len(token)-1 {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	body, err := base64.RawURLEncoding.DecodeString(token[:dot])
	if err != nil {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	signature, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if err != nil {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	mac := hmac.New(sha256.New, c.key)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	var envelope cursorEnvelope
	if json.Unmarshal(body, &envelope) != nil || envelope.Version != cursorVersion || envelope.Scope != expectedScope || len(envelope.Payload) > maxCursorPayload {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor is invalid", errInvalidCursor)
	}
	if !c.clock.Now().Before(time.Unix(envelope.ExpiresAt, 0)) {
		return shared.WrapError(shared.CodeExpired, "cursor has expired", errInvalidCursor)
	}
	if destination == nil || json.Unmarshal(envelope.Payload, destination) != nil {
		return shared.WrapError(shared.CodeInvalidArgument, "cursor payload is invalid", errInvalidCursor)
	}
	return nil
}
