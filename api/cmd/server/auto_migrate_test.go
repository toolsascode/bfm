package main

import (
	"testing"
	"time"

	"github.com/toolsascode/bfm/api/internal/backends"
)

func Test_autoMigrateRetryInterval(t *testing.T) {
	t.Run("default when unset", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_INTERVAL", "")
		if got := autoMigrateRetryInterval(); got != 5*time.Second {
			t.Fatalf("default = %v, want 5s", got)
		}
	})
	t.Run("explicit duration", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_INTERVAL", "3s")
		if got := autoMigrateRetryInterval(); got != 3*time.Second {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("zero means single-round mode", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_INTERVAL", "0s")
		if got := autoMigrateRetryInterval(); got != 0 {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("invalid falls back to 5s", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_INTERVAL", "not-a-duration")
		if got := autoMigrateRetryInterval(); got != 5*time.Second {
			t.Fatalf("got %v", got)
		}
	})
}

func Test_autoMigrateRetryMaxRounds(t *testing.T) {
	t.Run("zero interval forces 1", func(t *testing.T) {
		if n := autoMigrateRetryMaxRounds(0); n != 1 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("negative interval forces 1", func(t *testing.T) {
		if n := autoMigrateRetryMaxRounds(-1 * time.Second); n != 1 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("default when unset and interval positive", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS", "")
		if n := autoMigrateRetryMaxRounds(5 * time.Second); n != 24 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("explicit value", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS", "7")
		if n := autoMigrateRetryMaxRounds(5 * time.Second); n != 7 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("invalid falls back to 24", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS", "nope")
		if n := autoMigrateRetryMaxRounds(5 * time.Second); n != 24 {
			t.Fatalf("got %d", n)
		}
	})
	t.Run("zero env falls back to 24", func(t *testing.T) {
		t.Setenv("BFM_AUTO_MIGRATE_RETRY_MAX_ROUNDS", "0")
		if n := autoMigrateRetryMaxRounds(5 * time.Second); n != 24 {
			t.Fatalf("got %d", n)
		}
	})
}

func Test_connectionConfigReadyForAutoMigrate(t *testing.T) {
	tests := []struct {
		name string
		conn *backends.ConnectionConfig
		want bool
	}{
		{
			name: "nil config",
			conn: nil,
			want: false,
		},
		{
			name: "postgresql host set",
			conn: &backends.ConnectionConfig{Backend: "postgresql", Host: "db", Port: "5432"},
			want: true,
		},
		{
			name: "postgresql host empty",
			conn: &backends.ConnectionConfig{Backend: "postgresql", Host: "", Port: "5432"},
			want: false,
		},
		{
			name: "postgresql host whitespace",
			conn: &backends.ConnectionConfig{Backend: "postgresql", Host: "   "},
			want: false,
		},
		{
			name: "greptimedb host set",
			conn: &backends.ConnectionConfig{Backend: "greptimedb", Host: "gt", Port: ""},
			want: true,
		},
		{
			name: "greptimedb host empty",
			conn: &backends.ConnectionConfig{Backend: "greptimedb", Host: ""},
			want: false,
		},
		{
			name: "etcd endpoints lowercase",
			conn: &backends.ConnectionConfig{Backend: "etcd", Extra: map[string]string{"endpoints": "http://etcd:2379"}},
			want: true,
		},
		{
			name: "etcd endpoints uppercase key from env-style extra",
			conn: &backends.ConnectionConfig{Backend: "etcd", Extra: map[string]string{"ENDPOINTS": "http://etcd:2379"}},
			want: true,
		},
		{
			name: "etcd endpoints empty string",
			conn: &backends.ConnectionConfig{Backend: "etcd", Extra: map[string]string{"endpoints": "  "}},
			want: false,
		},
		{
			name: "etcd host and port",
			conn: &backends.ConnectionConfig{Backend: "etcd", Host: "etcd", Port: "2379"},
			want: true,
		},
		{
			name: "etcd METADATA_BACKEND only no host port endpoints",
			conn: &backends.ConnectionConfig{Backend: "etcd", Host: "", Port: "", Extra: map[string]string{}},
			want: false,
		},
		{
			name: "etcd host only",
			conn: &backends.ConnectionConfig{Backend: "etcd", Host: "etcd", Port: ""},
			want: false,
		},
		{
			name: "unknown backend passes",
			conn: &backends.ConnectionConfig{Backend: "futuredb", Host: ""},
			want: true,
		},
		{
			name: "backend casing",
			conn: &backends.ConnectionConfig{Backend: "PostgreSQL", Host: "x"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectionConfigReadyForAutoMigrate(tt.conn); got != tt.want {
				t.Errorf("connectionConfigReadyForAutoMigrate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_etcdEndpointsExtraNonEmpty(t *testing.T) {
	if !etcdEndpointsExtraNonEmpty(map[string]string{"endpoints": "a"}) {
		t.Fatal("expected true")
	}
	if etcdEndpointsExtraNonEmpty(map[string]string{"endpoints": ""}) {
		t.Fatal("expected false for empty value")
	}
	if etcdEndpointsExtraNonEmpty(nil) {
		t.Fatal("expected false for nil")
	}
}
