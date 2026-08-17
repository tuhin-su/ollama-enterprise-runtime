package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTokenAuthTestRouter(token string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Simulate what tokenAuthMiddleware does but with an explicit token
	// so we don't need to touch ~/.loom/server.json during tests.
	r.Use(func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}
		if c.Request.URL.Path == "/" {
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid Authorization header"})
			return
		}
		if auth[len(prefix):] != token {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API token"})
			return
		}
		c.Next()
	})

	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "Loom is running") })
	r.GET("/api/tags", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"models": []string{}}) })

	return r
}

func TestTokenAuth_NoTokenConfigured(t *testing.T) {
	r := setupTokenAuthTestRouter("") // no token → auth disabled

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tags", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no token is configured, got %d", w.Code)
	}
}

func TestTokenAuth_HealthCheckExempt(t *testing.T) {
	r := setupTokenAuthTestRouter("secret-123") // token set

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil) // health check
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for health check even with token set, got %d", w.Code)
	}
}

func TestTokenAuth_MissingHeader(t *testing.T) {
	r := setupTokenAuthTestRouter("secret-123")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tags", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without Authorization header, got %d", w.Code)
	}
}

func TestTokenAuth_WrongToken(t *testing.T) {
	r := setupTokenAuthTestRouter("secret-123")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tags", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}
}

func TestTokenAuth_ValidToken(t *testing.T) {
	r := setupTokenAuthTestRouter("secret-123")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/tags", nil)
	req.Header.Set("Authorization", "Bearer secret-123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token, got %d", w.Code)
	}
}
