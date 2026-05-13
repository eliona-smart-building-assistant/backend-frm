package httputil

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ElionaJWT struct {
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

// NewAuthorizationMiddleware returns a middleware that performs checks for presense of `elionaAuthorization` cookie
// or Authorization header (Bearer) and parses it as JWT using provided signing key.
//
// It sets `claims` value to request's context which is instance of [ElionaJWT]
func NewAuthorizationMiddleware(jwtSigningKey []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := findAuthToken(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			claims, err := parseJWT(jwtSigningKey, token)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "claims", claims)))
		})
	}
}

type AccessCheckMiddleware struct{}
