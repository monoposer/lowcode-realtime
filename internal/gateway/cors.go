package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware allows the standalone static Playground to call POST /api/event cross-origin.
// If origins is empty or contains only "*", Access-Control-Allow-Origin is set to *.
// Otherwise the request Origin is echoed only when it appears in the comma-separated allowlist.
func CORSMiddleware(origins []string) gin.HandlerFunc {
	useWildcard := len(origins) == 0
	if len(origins) == 1 && strings.TrimSpace(origins[0]) == "*" {
		useWildcard = true
	}

	return func(c *gin.Context) {
		if useWildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else {
			reqOrigin := c.GetHeader("Origin")
			for _, o := range origins {
				o = strings.TrimSpace(o)
				if o != "" && o == reqOrigin {
					c.Header("Access-Control-Allow-Origin", o)
					break
				}
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ParseCORSOrigins parses the env value: empty or "*" yields nil (wildcard); otherwise split by comma.
func ParseCORSOrigins(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "*" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
