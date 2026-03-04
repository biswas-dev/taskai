package api

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"taskai/internal/auth"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for user ID
	UserIDKey contextKey = "user_id"
	// UserEmailKey is the context key for user email
	UserEmailKey contextKey = "user_email"
)

// JWTAuth middleware validates JWT tokens or API keys from Authorization header
func (s *Server) JWTAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			respondError(w, http.StatusUnauthorized, "missing authorization header", "unauthorized")
			return
		}

		// Check for Bearer token format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 {
			respondError(w, http.StatusUnauthorized, "invalid authorization header format", "unauthorized")
			return
		}

		authType := parts[0]
		credential := parts[1]

		var userID int64
		var email string
		var err error

		switch authType {
		case "Bearer":
			// JWT token authentication
			claims, jwtErr := auth.ValidateToken(credential, s.config.JWTSecret)
			if jwtErr != nil {
				s.logger.Warn("Token validation failed", zap.Error(jwtErr))
				respondError(w, http.StatusUnauthorized, "invalid or expired token", "unauthorized")
				return
			}
			userID = claims.UserID
			email = claims.Email

			// Reject tokens for soft-deleted users
			if deleted, _ := s.db.IsUserDeleted(r.Context(), userID); deleted {
				respondError(w, http.StatusUnauthorized, "invalid or expired token", "unauthorized")
				return
			}

		case "ApiKey":
			// API key authentication
			userID, email, err = s.db.GetUserByAPIKey(r.Context(), credential)
			if err != nil {
				s.logger.Warn("API key validation failed", zap.Error(err))
				respondError(w, http.StatusUnauthorized, "invalid or expired API key", "unauthorized")
				return
			}

		default:
			respondError(w, http.StatusUnauthorized, "unsupported authorization type", "unauthorized")
			return
		}

		// Add user info to request context
		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		ctx = context.WithValue(ctx, UserEmailKey, email)

		// Continue to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ZapLogger returns a middleware that logs HTTP requests using the provided zap logger.
func ZapLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.Info("HTTP request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.statusCode),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so WebSocket upgrades work through this wrapper.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

// Flush implements http.Flusher so SSE (Server-Sent Events) works through this wrapper.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for http.NewResponseController compatibility.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// GetUserID extracts user ID from request context
func GetUserID(r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(UserIDKey).(int64)
	return userID, ok
}

// GetUserEmail extracts user email from request context
func GetUserEmail(r *http.Request) (string, bool) {
	email, ok := r.Context().Value(UserEmailKey).(string)
	return email, ok
}

// tokenBucket implements a simple token bucket rate limiter
type tokenBucket struct {
	tokens      float64
	capacity    float64
	refillRate  float64
	lastRefill  time.Time
	mu          sync.Mutex
}

func newTokenBucket(capacity, refillRate float64) *tokenBucket {
	return &tokenBucket{
		tokens:     capacity,
		capacity:   capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()

	// Refill tokens based on elapsed time
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}
	tb.lastRefill = now

	// Check if we have tokens available
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}

	return false
}

// rateLimiter manages rate limits for different endpoints
type rateLimiter struct {
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		buckets: make(map[string]*tokenBucket),
	}
}

func (rl *rateLimiter) getBucket(key string, capacity, refillRate float64) *tokenBucket {
	rl.mu.RLock()
	bucket, exists := rl.buckets[key]
	rl.mu.RUnlock()

	if exists {
		return bucket
	}

	// Create new bucket
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	bucket, exists = rl.buckets[key]
	if exists {
		return bucket
	}

	bucket = newTokenBucket(capacity, refillRate)
	rl.buckets[key] = bucket
	return bucket
}

// RateLimitMiddleware creates a rate limiting middleware
func RateLimitMiddleware(requestsPerMinute int) func(http.Handler) http.Handler {
	limiter := newRateLimiter()
	capacity := float64(requestsPerMinute)
	refillRate := capacity / 60.0 // tokens per second

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use IP address as key
			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}
			if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				ip = realIP
			}

			bucket := limiter.getBucket(ip, capacity, refillRate)
			if !bucket.allow() {
				respondError(w, http.StatusTooManyRequests, "rate limit exceeded", "rate_limit_exceeded")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
