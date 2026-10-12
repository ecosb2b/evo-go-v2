package send_handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
)

func TestWriteSendError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not connected", whatsmeow_service.ErrInstanceNotConnected, http.StatusConflict},
		{"wrapped not connected", fmt.Errorf("send: %w", whatsmeow_service.ErrInstanceNotConnected), http.StatusConflict},
		{"other failure", errors.New("upload failed"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(rec)

			writeSendError(ctx, tt.err)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
