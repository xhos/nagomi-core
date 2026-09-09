package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// UserContext middleware automatically extracts and sets user ID in context
func UserContext() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Check if this is an internal service request
			if isInternalAuth := ctx.Value(InternalAuthKey); isInternalAuth != nil {
				// For internal services, extract user_id from the request body
				if userID := extractUserIDFromRequest(r); userID != "" {
					if parsedID, err := uuid.Parse(userID); err == nil {
						ctx = context.WithValue(ctx, UserIDKey, parsedID)
						r = r.WithContext(ctx)
					}
				}
			} else {
				// For regular users, extract from JWT context
				if user, ok := ctx.Value(UserContextKey).(*User); ok {
					if parsedID, err := uuid.Parse(user.ID); err == nil {
						ctx = context.WithValue(ctx, UserIDKey, parsedID)
						r = r.WithContext(ctx)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractUserIDFromRequest extracts user_id from Connect-RPC request body
func extractUserIDFromRequest(r *http.Request) string {
	// Only process Connect-RPC requests
	if !strings.Contains(r.URL.Path, "/nagomi.v1.") {
		return ""
	}

	// Skip health checks and user service requests that don't need user_id
	if strings.Contains(r.URL.Path, "grpc.health") ||
		strings.Contains(r.URL.Path, "/nagomi.v1.UserService/GetUser") {
		return ""
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}

	// Restore the body for the next handler
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Try JSON first (for Connect protocol with JSON)
	var requestData map[string]interface{}
	if err := json.Unmarshal(body, &requestData); err == nil {
		if userID, ok := requestData["userId"].(string); ok && userID != "" {
			return userID
		}
		if userID, ok := requestData["user_id"].(string); ok && userID != "" {
			return userID
		}
	}

	// For protobuf binary requests, we can't easily parse them here
	// Instead, we'll need a different approach - let's use a Connect interceptor
	// For now, return empty and let the handler deal with it
	return ""
}

// UserIDExtractor creates a Connect interceptor that extracts user IDs from requests
func UserIDExtractor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Check if this is an internal service request
			if isInternalAuth := ctx.Value(InternalAuthKey); isInternalAuth != nil {
				// For internal services, extract user_id from the request message
				if userID := extractUserIDFromMessage(req.Any()); userID != "" {
					parsedID, err := uuid.Parse(userID)
					if err == nil {
						ctx = context.WithValue(ctx, UserIDKey, parsedID)
					}
				}
			} else {
				// For regular users, extract from JWT context
				if user, ok := ctx.Value(UserContextKey).(*User); ok {
					if parsedID, err := uuid.Parse(user.ID); err == nil {
						ctx = context.WithValue(ctx, UserIDKey, parsedID)
					}
				}
			}

			return next(ctx, req)
		}
	}
}

// extractUserIDFromMessage extracts user_id field from any protobuf message using reflection
func extractUserIDFromMessage(msg any) string {
	if msg == nil {
		return ""
	}

	val := reflect.ValueOf(msg)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return ""
	}

	// Look for GetUserId method (common protobuf pattern)
	msgType := val.Type()
	for i := 0; i < msgType.NumMethod(); i++ {
		method := msgType.Method(i)
		if method.Name == "GetUserId" {
			result := val.Method(i).Call(nil)
			if len(result) == 1 && result[0].Kind() == reflect.String {
				return result[0].String()
			}
		}
	}

	// Fallback: look for user_id field directly
	userIdField := val.FieldByName("UserId")
	if userIdField.IsValid() && userIdField.Kind() == reflect.String {
		return userIdField.String()
	}

	return ""
}
