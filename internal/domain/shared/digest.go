package shared

import (
	"crypto/sha256"
	"encoding/hex"
)

const sha256Prefix = "sha256:"

// SHA256Digest is an immutable SHA-256 content identity.
type SHA256Digest [sha256.Size]byte

func DigestBytes(content []byte) SHA256Digest { return sha256.Sum256(content) }

func ParseSHA256Digest(value string) (SHA256Digest, error) {
	var digest SHA256Digest
	if len(value) != len(sha256Prefix)+hex.EncodedLen(len(digest)) || value[:len(sha256Prefix)] != sha256Prefix {
		return digest, NewError(CodeInvalidArgument, "digest must use canonical sha256 form")
	}
	decoded, err := hex.DecodeString(value[len(sha256Prefix):])
	if err != nil {
		return digest, WrapError(CodeInvalidArgument, "digest contains invalid hexadecimal", err)
	}
	copy(digest[:], decoded)
	if digest.String() != value {
		return SHA256Digest{}, NewError(CodeInvalidArgument, "digest must use lowercase hexadecimal")
	}
	return digest, nil
}

func (d SHA256Digest) String() string { return sha256Prefix + hex.EncodeToString(d[:]) }

func (d SHA256Digest) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func (d *SHA256Digest) UnmarshalText(text []byte) error {
	parsed, err := ParseSHA256Digest(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
