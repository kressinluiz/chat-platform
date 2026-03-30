package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

type contextKey string

const claimsKey contextKey = "claims"

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			WriteError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			WriteError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		claims, err := ValidateToken(parts[1])
		if err != nil {
			slog.Warn("invalid token on http request", "error", err)
			WriteError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		r = SetClaims(r, claims)
		next(w, r)
	}
}

func SetClaims(r *http.Request, claims *Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, claims))
}

func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}
