package middleware

import (
	"net/http"
	"strings"

	"github.com/bok1c4/pwman/internal/api"
)

func Auth(authManager *api.AuthManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/unlock" ||
			r.URL.Path == "/api/init" ||
			r.URL.Path == "/api/is_initialized" ||
			r.URL.Path == "/api/ping" ||
			r.URL.Path == "/api/health" {
			next(w, r)
			return
		}

		token := r.Header.Get("Authorization")
		if token == "" {
			api.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required")
			return
		}

		if strings.HasPrefix(token, "Bearer ") {
			token = strings.TrimPrefix(token, "Bearer ")
		}

		if !authManager.ValidateToken(token) {
			api.Error(w, http.StatusUnauthorized, "INVALID_TOKEN", "invalid or expired token")
			return
		}

		next(w, r)
	}
}
