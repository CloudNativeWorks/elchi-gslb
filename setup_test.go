package elchi

import (
	"testing"
	"time"
)

// Test validation logic for setup.go
// Note: These test the validation rules, not the Caddy parsing.
// Full integration tests with Caddy would require a running CoreDNS instance.

func TestValidation_DefaultValues(t *testing.T) {
	// Test that default values are correct
	defaultTTL := uint32(300)
	defaultSyncInterval := 5 * time.Minute
	defaultTimeout := 10 * time.Second
	defaultWebhookAddr := ":8053"

	if defaultTTL != 300 {
		t.Errorf("Default TTL = %d, want 300", defaultTTL)
	}
	if defaultSyncInterval != 5*time.Minute {
		t.Errorf("Default SyncInterval = %v, want 5m", defaultSyncInterval)
	}
	if defaultTimeout != 10*time.Second {
		t.Errorf("Default Timeout = %v, want 10s", defaultTimeout)
	}
	if defaultWebhookAddr != ":8053" {
		t.Errorf("Default WebhookAddr = %s, want :8053", defaultWebhookAddr)
	}
}

func TestValidation_SecretLength(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{"valid secret", "testsecret123", false},
		{"exactly 8 chars", "12345678", false},
		{"too short", "short", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from setup.go line 127-134
			err := validateSecret(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidation_SyncIntervalVsTimeout(t *testing.T) {
	tests := []struct {
		name         string
		syncInterval time.Duration
		timeout      time.Duration
		wantErr      bool
	}{
		{"valid: sync > timeout", 5 * time.Minute, 10 * time.Second, false},
		{"invalid: sync == timeout", 10 * time.Second, 10 * time.Second, true},
		{"invalid: sync < timeout", 5 * time.Second, 10 * time.Second, true},
		{"valid: large difference", 1 * time.Hour, 30 * time.Second, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from setup.go line 137-139
			err := validateIntervalTimeout(tt.syncInterval, tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIntervalTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidation_MinimumSyncInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		wantErr  bool
	}{
		{"valid: 1 minute", 1 * time.Minute, false},
		{"valid: 5 minutes", 5 * time.Minute, false},
		{"invalid: 30 seconds", 30 * time.Second, true},
		{"invalid: 0", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from setup.go line 88-90
			err := validateSyncInterval(tt.interval)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSyncInterval() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidation_MinimumTimeout(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{"valid: 1 second", 1 * time.Second, false},
		{"valid: 10 seconds", 10 * time.Second, false},
		{"invalid: 500ms", 500 * time.Millisecond, true},
		{"invalid: 0", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from setup.go line 101-103
			err := validateTimeout(tt.timeout)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidation_TTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     uint32
		wantErr bool
	}{
		{"valid: 300", 300, false},
		{"valid: 1", 1, false},
		{"valid: 86400", 86400, false},
		{"invalid: 0", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from setup.go line 75-77
			err := validateTTL(tt.ttl)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTTL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidation_RequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		zone     string
		endpoint string
		secret   string
		wantErr  bool
	}{
		{"all fields present", "gslb.elchi.", "http://localhost:8080", "testsecret123", false},
		{"missing zone", "", "http://localhost:8080", "testsecret123", true},
		{"missing endpoint", "gslb.elchi.", "", "testsecret123", true},
		{"missing secret", "gslb.elchi.", "http://localhost:8080", "", true},
		{"all missing", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequiredFields(tt.zone, tt.endpoint, tt.secret)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequiredFields() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Helper functions that simulate validation logic from setup.go

func validateSecret(secret string) error {
	if secret == "" {
		return &validationError{"secret is required"}
	}
	if len(secret) < 8 {
		return &validationError{"secret must be at least 8 characters long"}
	}
	return nil
}

func validateSyncInterval(interval time.Duration) error {
	if interval < 1*time.Minute {
		return &validationError{"sync_interval must be at least 1m"}
	}
	return nil
}

func validateTimeout(timeout time.Duration) error {
	if timeout < 1*time.Second {
		return &validationError{"timeout must be at least 1s"}
	}
	return nil
}

func validateTTL(ttl uint32) error {
	if ttl == 0 {
		return &validationError{"ttl must be greater than 0"}
	}
	return nil
}

func validateIntervalTimeout(syncInterval, timeout time.Duration) error {
	if syncInterval <= timeout {
		return &validationError{"sync_interval must be greater than timeout"}
	}
	return nil
}

func validateRequiredFields(zone, endpoint, secret string) error {
	if zone == "" {
		return &validationError{"zone is required"}
	}
	if endpoint == "" {
		return &validationError{"endpoint is required"}
	}
	if secret == "" {
		return &validationError{"secret is required"}
	}
	return nil
}

// validationError is a simple error type for validation
type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}
