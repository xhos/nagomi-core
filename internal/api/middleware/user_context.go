package middleware

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

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

// extractUserIDFromMessage reads the user_id field of request messages that have one
func extractUserIDFromMessage(msg any) string {
	if m, ok := msg.(interface{ GetUserId() string }); ok {
		return m.GetUserId()
	}
	return ""
}
