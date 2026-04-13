package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kressinluiz/chat/internal/auth"
	"github.com/kressinluiz/chat/internal/handler"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type contextKey string

const claimsKey contextKey = "claims"

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			handler.WriteError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			handler.WriteError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		claims, err := auth.ValidateToken(parts[1])
		if err != nil {
			slog.Warn("invalid token on http request", "error", err)
			handler.WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		r = SetClaims(r, claims)
		next(w, r)
	}
}

func SetClaims(r *http.Request, claims *auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, claims))
}

func GetClaims(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return claims
}

func TracingMiddleware(next http.Handler, name string) http.Handler {
	return otelhttp.NewHandler(next, name)
}
