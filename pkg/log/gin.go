package log

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const headerRequestID = "X-Request-ID"

// GinMiddleware returns a Gin middleware that:
//  1. Generates or reads a request ID from X-Request-ID header.
//  2. Creates a child logger with request metadata and injects it into context.
//  3. Sets the X-Request-ID response header.
//  4. Logs the completed request with status, latency, and actor info.
func GinMiddleware(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqID := c.GetHeader(headerRequestID)
		if reqID == "" {
			reqID = uuid.New().String()
		}

		child := logger.With().
			Str(FieldRequestID, reqID).
			Str(FieldMethod, c.Request.Method).
			Str(FieldPath, c.Request.URL.Path).
			Str(FieldClientIP, c.ClientIP()).
			Logger()

		c.Header(headerRequestID, reqID)
		c.Request = c.Request.WithContext(WithLogger(c.Request.Context(), child))

		c.Next()

		// Read actor info set by auth middleware after c.Next().
		evt := child.Info().
			Int(FieldStatus, c.Writer.Status()).
			Float64(FieldLatency, float64(time.Since(start).Milliseconds()))

		if userID, ok := c.Get(FieldUserID); ok {
			evt = evt.Str(FieldUserID, userID.(string))
		}
		if username, ok := c.Get(FieldUsername); ok {
			evt = evt.Str(FieldUsername, username.(string))
		}

		evt.Msg("request completed")
	}
}
