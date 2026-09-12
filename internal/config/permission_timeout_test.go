package config

import (
	"testing"
	"time"
)

func TestResolvedPermissionTimeout(t *testing.T) {
	tests := []struct {
		name string
		secs int
		want time.Duration
	}{
		{"unset waits forever", 0, 0},
		{"positive bounds the wait", 90, 90 * time.Second},
		{"negative treated as unset", -5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := &Tools{PermissionTimeoutSeconds: tt.secs}
			if got := tools.ResolvedPermissionTimeout(); got != tt.want {
				t.Fatalf("ResolvedPermissionTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRejectsNegativePermissionTimeout(t *testing.T) {
	tools := &Tools{PermissionTimeoutSeconds: -1}
	if err := tools.Validate(); err == nil {
		t.Fatal("expected validation error for negative permission_timeout_seconds")
	}
}
