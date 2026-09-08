package middleware

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

type AuthConfig struct {
	InternalAPIKey string
	WebURL         string
}

func validateJWT(ctx context.Context, tokenString, webURL string) (*User, error) {
	if webURL == "" {
		return nil, fmt.Errorf("web URL not configured")
	}

	jwksURL := fmt.Sprintf("%s/api/auth/jwks", webURL)

	keyset, err := jwk.Fetch(ctx, jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS from %s: %w", jwksURL, err)
	}

	token, err := jwt.Parse([]byte(tokenString), jwt.WithKeySet(keyset))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	userID, exists := token.Subject()
	if !exists {
		return nil, errors.New("missing user id")
	}

	var email, name string
	_ = token.Get("email", &email)
	_ = token.Get("name", &name)

	return &User{
		ID:    userID,
		Email: email,
		Name:  name,
	}, nil
}

func Auth(config *AuthConfig, logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/grpc.health.v1.Health") {
				next.ServeHTTP(w, r)
				return
			}

			if internalKey := r.Header.Get("X-Internal-Key"); internalKey != "" {
				if config.InternalAPIKey == "" {
					logger.Error("internal API key not configured")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if subtle.ConstantTimeCompare([]byte(internalKey), []byte(config.InternalAPIKey)) != 1 {
					logger.Warn("invalid internal API key", "remote_addr", r.RemoteAddr)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				ctx := context.WithValue(r.Context(), InternalAuthKey, true)
				logger.Debug("internal service authenticated", "path", r.URL.Path)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			authHeader := r.Header.Get("Authorization")
			if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
				tokenString := after

				if config.WebURL == "" {
					logger.Error("web URL not configured")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				user, err := validateJWT(r.Context(), tokenString, config.WebURL)
				if err != nil {
					logger.Warn("JWT validation failed", "error", err, "remote_addr", r.RemoteAddr)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}

				ctx := context.WithValue(r.Context(), UserContextKey, user)
				logger.Debug("user authenticated via JWT",
					"user_id", user.ID,
					"email", user.Email,
					"path", r.URL.Path,
				)

				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			logger.Warn("unauthorized request", "path", r.URL.Path, "remote_addr", r.RemoteAddr)
			w.WriteHeader(http.StatusUnauthorized)
		})
	}
}
