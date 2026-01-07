package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ha1tch/olu/pkg/config"
)

// ContextKey type for context values
type ContextKey string

const (
	// ContextKeySubject is the key for the authenticated subject
	ContextKeySubject ContextKey = "auth_subject"
	// ContextKeyAuthMethod is the key for the auth method used
	ContextKeyAuthMethod ContextKey = "auth_method"
)

// AuthMiddleware creates an authentication middleware based on config
func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for excluded paths
			for _, path := range cfg.AuthExcludePaths {
				if strings.HasPrefix(r.URL.Path, path) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// No auth configured
			if cfg.AuthType == "none" || cfg.AuthType == "" {
				next.ServeHTTP(w, r)
				return
			}

			var subject string
			var authMethod string
			var authenticated bool

			switch cfg.AuthType {
			case "jwt":
				subject, authenticated = validateJWT(r, cfg)
				authMethod = "jwt"
			case "apikey":
				subject, authenticated = validateAPIKey(r, cfg)
				authMethod = "apikey"
			default:
				// Unknown auth type, allow through (fail open for backwards compat)
				next.ServeHTTP(w, r)
				return
			}

			if !authenticated {
				writeAuthError(w, cfg.AuthType)
				return
			}

			// Add auth info to context
			ctx := context.WithValue(r.Context(), ContextKeySubject, subject)
			ctx = context.WithValue(ctx, ContextKeyAuthMethod, authMethod)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateJWT validates a JWT token from Authorization header
func validateJWT(r *http.Request, cfg *config.Config) (string, bool) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", false
	}

	token := parts[1]

	// Parse and validate JWT (simplified HS256 validation)
	claims, valid := parseAndValidateJWT(token, cfg.JWTSecret, cfg.JWTIssuer)
	if !valid {
		return "", false
	}

	subject, _ := claims["sub"].(string)
	return subject, true
}

// JWTClaims represents standard JWT claims
type JWTClaims map[string]interface{}

// parseAndValidateJWT parses and validates a JWT token
// Supports HS256 algorithm only for simplicity
func parseAndValidateJWT(token, secret, expectedIssuer string) (JWTClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}

	// Decode header
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, false
	}
	var header map[string]interface{}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, false
	}

	// Check algorithm
	if alg, ok := header["alg"].(string); !ok || alg != "HS256" {
		return nil, false
	}

	// Decode payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims JWTClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, false
	}

	// Verify signature
	signatureInput := parts[0] + "." + parts[1]
	expectedSig := computeHS256(signatureInput, secret)
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, false
	}
	if !hmac.Equal(expectedSig, actualSig) {
		return nil, false
	}

	// Check expiration
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, false
		}
	}

	// Check not before
	if nbf, ok := claims["nbf"].(float64); ok {
		if time.Now().Unix() < int64(nbf) {
			return nil, false
		}
	}

	// Check issuer if configured
	if expectedIssuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != expectedIssuer {
			return nil, false
		}
	}

	return claims, true
}

// computeHS256 computes HMAC-SHA256 signature
func computeHS256(input, secret string) []byte {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(input))
	return h.Sum(nil)
}

// validateAPIKey validates an API key from header or query param
func validateAPIKey(r *http.Request, cfg *config.Config) (string, bool) {
	// Check X-API-Key header first
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		// Check Authorization header with ApiKey scheme
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "ApiKey ") {
			apiKey = strings.TrimPrefix(authHeader, "ApiKey ")
		}
	}
	if apiKey == "" {
		// Check query parameter as fallback
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey == "" {
		return "", false
	}

	// Validate against configured keys
	for _, validKey := range cfg.APIKeys {
		if validKey != "" && apiKey == validKey {
			// Use key prefix as subject (first 8 chars)
			subject := apiKey
			if len(subject) > 8 {
				subject = subject[:8] + "..."
			}
			return subject, true
		}
	}

	return "", false
}

// writeAuthError writes an authentication error response
func writeAuthError(w http.ResponseWriter, authType string) {
	w.Header().Set("Content-Type", "application/json")

	switch authType {
	case "jwt":
		w.Header().Set("WWW-Authenticate", `Bearer realm="olu"`)
	case "apikey":
		w.Header().Set("WWW-Authenticate", `ApiKey realm="olu"`)
	}

	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "Unauthorized",
		"message": "Authentication required",
	})
}

// GetSubject retrieves the authenticated subject from context
func GetSubject(ctx context.Context) string {
	if subject, ok := ctx.Value(ContextKeySubject).(string); ok {
		return subject
	}
	return ""
}

// GetAuthMethod retrieves the auth method from context
func GetAuthMethod(ctx context.Context) string {
	if method, ok := ctx.Value(ContextKeyAuthMethod).(string); ok {
		return method
	}
	return ""
}
