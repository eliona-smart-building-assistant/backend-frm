package httputil

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ElionaJWT struct {
	Audit        string `json:"aud"`
	ExpiresAt    int    `json:"exp"`
	IssuedBy     string `json:"iss"`
	Role         string `json:"role"`
	RoleID       string `json:"role_id"`
	UserID       string `json:"user_id"`
	TenantID     string `json:"tenant_id"`
	Entitlements string `json:"entitlements"`
	jwt.RegisteredClaims
}

func parseJWT(key []byte, tokenString string) (*ElionaJWT, error) {
	token, err := jwt.ParseWithClaims(tokenString, &ElionaJWT{}, func(token *jwt.Token) (any, error) {
		return key, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*ElionaJWT)
	if !ok || claims == nil {
		return nil, err
	}

	return claims, nil
}

func findAuthToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	token := tokenFromHeader(authHeader)

	if len(token) > 0 {
		return token, nil
	}

	cookie, err := r.Cookie("elionaAuthorization")
	if err != nil {
		return "", err
	}

	return cookie.Value, nil
}

func tokenFromHeader(header string) string {
	if len(header) == 0 {
		return ""
	}

	parts := strings.Split(header, " ")
	if len(parts) != 2 {
		return ""
	}

	if strings.ToLower(parts[0]) != "bearer" {
		return ""
	}

	return parts[1]
}

type AuthorizationMiddleware struct {
	key  []byte
	next http.Handler
}

func NewAuthorizationMiddleware(signingKey []byte, next http.Handler) AuthorizationMiddleware {
	return AuthorizationMiddleware{key: signingKey, next: next}
}

func (m AuthorizationMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, err := findAuthToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	claims, err := parseJWT(m.key, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	m.next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "claims", claims)))
}

type AccessCheckMiddleware struct{}
