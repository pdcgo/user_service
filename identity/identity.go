package identity

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v4"
	role_base "github.com/pdcgo/schema/services/role_base/v1"
	"google.golang.org/protobuf/proto"
)

// TokenIdentity is the user_service login-token payload. It carries a
// role_base.Identity and serializes it into an HS256 JWT.
type TokenIdentity struct {
	*role_base.Identity
}

// tokenClaims matches pkgs/san_collection/rolebased.Claims wire format so the
// token is decodable by rolebased.ExtractRoleFromRequest (header x-role-identity).
type tokenClaims struct {
	Data []byte `json:"d"`
	jwt.RegisteredClaims
}

// Serialize proto-marshals the embedded Identity and signs it as an HS256 JWT.
func (t *TokenIdentity) Serialize(secret string) (string, error) {
	raw, err := proto.Marshal(t.Identity)
	if err != nil {
		return "", err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &tokenClaims{Data: raw})
	return token.SignedString([]byte(secret))
}

// Parse verifies the HS256 signature with secret and decodes the embedded
// role_base.Identity. It does NOT check expiry (see IsExpired).
func Parse(secret, tokenString string) (*TokenIdentity, error) {
	var claims tokenClaims
	_, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	id := &role_base.Identity{}
	if err := proto.Unmarshal(claims.Data, id); err != nil {
		return nil, err
	}

	return &TokenIdentity{Identity: id}, nil
}

// IsExpired reports whether the identity's ExpiredAt is at or before now. A
// missing Identity or ExpiredAt is treated as expired.
func (t *TokenIdentity) IsExpired(now time.Time) bool {
	if t.Identity == nil || t.Identity.ExpiredAt == nil {
		return true
	}
	return !t.Identity.ExpiredAt.AsTime().After(now)
}
