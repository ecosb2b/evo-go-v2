package instance_repository

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

func dryRunRepo(t *testing.T) (InstanceRepository, *bytes.Buffer) {
	t.Helper()
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	var logBuffer bytes.Buffer
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DryRun: true,
		Logger: gormlogger.New(
			log.New(&logBuffer, "", 0),
			gormlogger.Config{LogLevel: gormlogger.Info, Colorful: false},
		),
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	return NewInstanceRepository(gormDB), &logBuffer
}

// O boot deve religar instâncias pareadas que caíram no meio de uma reconexão
// (connected=false), mas não as que foram desligadas de propósito pela API.
func TestGetInstancesToRestoreQuery(t *testing.T) {
	tests := []struct {
		name       string
		clientName string
		wantClient bool
	}{
		{"all clients", "", false},
		{"one client", "client-a", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, logs := dryRunRepo(t)

			if _, err := repo.GetInstancesToRestore(tt.clientName); err != nil {
				t.Fatalf("GetInstancesToRestore: %v", err)
			}

			sql := logs.String()
			for _, want := range []string{
				"jid <> ''",
				"connected = true",
				instance_model.DisconnectReasonByAPI,
				instance_model.DisconnectReasonLogoutByAPI,
			} {
				if !strings.Contains(sql, want) {
					t.Errorf("query missing %q:\n%s", want, sql)
				}
			}
			if got := strings.Contains(sql, "client_name"); got != tt.wantClient {
				t.Errorf("client_name filter present = %v, want %v:\n%s", got, tt.wantClient, sql)
			}
		})
	}
}
