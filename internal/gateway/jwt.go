package gateway

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrMissingToken is returned when JWT auth is required but no token was sent.
	ErrMissingToken = errors.New("missing token")
	// ErrInvalidToken is returned when the token cannot be parsed or validated.
	ErrInvalidToken = errors.New("invalid token")
)

// JWTAuth holds HS256 verification settings for WebSocket connections.
type JWTAuth struct {
	Secret    []byte
	UserClaim string // claim name for user id: "sub" (default) or "user_id"
}

// ParseUserIDFromRequest extracts the Bearer token from Authorization header or
// `token` / `access_token` query params, verifies HS256, and returns the user id
// from the configured claim (default `sub`, fallback `user_id` in map claims).
func (a *JWTAuth) ParseUserIDFromRequest(r *http.Request) (string, error) {
	if a == nil || len(a.Secret) == 0 {
		return "", ErrInvalidToken
	}
	raw := extractBearerOrQueryToken(r)
	if raw == "" {
		log.Printf("ws/jwt: no Bearer header or token/access_token query")
		return "", ErrMissingToken
	}

	log.Printf("ws/jwt: verifying token (len=%d, secret_len=%d, user_claim=%q)", len(raw), len(a.Secret), a.UserClaim)

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return a.Secret, nil
	})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claimName := a.UserClaim
	if claimName == "" {
		claimName = "sub"
	}

	// Prefer configured claim, then user_id
	if v, ok := claims[claimName].(string); ok && v != "" {
		log.Printf("ws/jwt: user id from claim %q", claimName)
		return v, nil
	}
	if v, ok := claims["user_id"].(string); ok && v != "" {
		log.Printf("ws/jwt: user id from claim user_id")
		return v, nil
	}
	if v, ok := claims["sub"].(string); ok && v != "" {
		return v, nil
	}

	log.Printf("ws/jwt: no string user id in claims (tried %q, user_id, sub); keys=%v", claimName, claimKeys(claims))
	return "", ErrInvalidToken
}

func claimKeys(m jwt.MapClaims) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func extractBearerOrQueryToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return r.URL.Query().Get("access_token")
}

// HasTokenInRequest reports whether the client sent a token (Bearer or token/access_token query).
func HasTokenInRequest(r *http.Request) bool {
	return extractBearerOrQueryToken(r) != ""
}
