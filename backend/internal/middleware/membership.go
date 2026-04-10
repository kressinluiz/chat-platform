package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kressinluiz/chat/internal/repository"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func RequireMembership(next http.HandlerFunc, repo repository.RoomMemberRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.URL.Query().Get("token")
		if tokenString == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		claims, err := ValidateToken(tokenString)
		if err != nil {
			slog.Warn("invalid token on websocket upgrade", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		roomID := r.URL.Query().Get("room_id")
		if roomID == "" {
			http.Error(w, "missing room_id", http.StatusUnauthorized)
			slog.Warn("missing room_id in query")
			return
		}

		isMember, err := repo.IsMember(r.Context(), roomID, claims.UserID)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			slog.Warn("error checking membership", "error", err)
			return
		}
		if !isMember {
			http.Error(w, "not a member of this room", http.StatusForbidden)
			slog.Warn("user is not a member of the requested room")
			return
		}

		next.ServeHTTP(w, r)
	}
}

func RequireRole(repo repository.RoomMemberRepo, minRole repository.RoomRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := r.URL.Query().Get("token")
			if tokenString == "" {
				http.Error(w, "missing token", http.StatusUnauthorized)
				return
			}
			claims, err := ValidateToken(tokenString)
			if err != nil {
				slog.Warn("invalid token on websocket upgrade", "error", err)
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			roomID := r.URL.Query().Get("room_id")
			if roomID == "" {
				slog.Warn("missing room_id")
				http.Error(w, "missing room_id", http.StatusUnauthorized)
				return
			}

			role, err := repo.GetRole(r.Context(), roomID, claims.UserID)
			if err != nil {
				slog.Warn("error fetching user role", "error", err)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			if !hasMinRole(role, minRole) {
				slog.Warn("user does not have required role", "user_id", claims.UserID, "room_id", roomID, "required_role", minRole, "actual_role", role)
				http.Error(w, "insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func hasMinRole(actual, required repository.RoomRole) bool {
	rank := map[repository.RoomRole]int{
		repository.RoleMember:    1,
		repository.RoleModerator: 2,
		repository.RoleAdmin:     3,
	}
	return rank[actual] >= rank[required]
}

func ValidateToken(tokenString string) (*Claims, error) {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JWT_SECRET is not set")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
