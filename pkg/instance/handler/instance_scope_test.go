package instance_handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func TestAuthorizeInstanceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		admin      bool
		instance   *instance_model.Instance
		pathID     string
		wantOK     bool
		wantStatus int
	}{
		{
			name:     "instance token on its own id",
			instance: &instance_model.Instance{Id: "inst-a"},
			pathID:   "inst-a",
			wantOK:   true,
		},
		{
			name:       "instance token on another instance's id",
			instance:   &instance_model.Instance{Id: "inst-a"},
			pathID:     "inst-b",
			wantOK:     false,
			wantStatus: http.StatusForbidden,
		},
		{
			name:   "global key on any id",
			admin:  true,
			pathID: "inst-b",
			wantOK: true,
		},
		{
			name:       "no credentials in context",
			pathID:     "inst-a",
			wantOK:     false,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)
			if tt.admin {
				ctx.Set("admin", true)
			}
			if tt.instance != nil {
				ctx.Set("instance", tt.instance)
			}

			ok := authorizeInstanceScope(ctx, tt.pathID)

			if ok != tt.wantOK {
				t.Fatalf("authorizeInstanceScope() = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK && rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
