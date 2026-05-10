package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORS_EKOAllowedOriginsOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("EKO_ALLOWED_ORIGINS", "https://demo.workers.dev, https://app.example.com")

	r := gin.New()
	r.Use(CORS())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	cases := []struct {
		origin string
		want   string // expected Access-Control-Allow-Origin echoed back
	}{
		{"https://demo.workers.dev", "https://demo.workers.dev"},
		{"https://app.example.com", "https://app.example.com"},
		{"https://not-allowed.example", ""}, // gin-contrib/cors omits the header for blocked origins
	}

	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/x", nil)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Access-Control-Request-Method", "GET")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			got := rr.Header().Get("Access-Control-Allow-Origin")
			if got != tc.want {
				t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, tc.want)
			}
		})
	}
}
