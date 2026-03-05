package api

import (
	"strings"
	"testing"
)

func TestValidateSite(t *testing.T) {
	tests := []struct {
		name      string
		site      string
		wantErr   bool
		expectErr error
	}{
		{"valid site", "github.com", false, nil},
		{"valid with subdomain", "sub.example.com", false, nil},
		{"valid with path", "example.com/path", false, nil},
		{"empty site", "", true, ErrInvalidSite},
		{"whitespace only", "   ", true, ErrInvalidSite},
		{"with spaces", "git hub.com", false, nil},
		{"with tab", "github\t.com", false, nil},
		{"with newline", "github\n.com", false, nil},
		{"with null char", "github\x00.com", false, nil},
		{"normalizes spaces", "  github.com  ", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateSite(tt.site)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSite() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != tt.expectErr {
				t.Errorf("ValidateSite() error = %v, want %v", err, tt.expectErr)
			}
			if !tt.wantErr && result == "" {
				t.Error("ValidateSite() returned empty string for valid input")
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"valid username", "john@example.com", false},
		{"valid simple", "user123", false},
		{"empty", "", false},
		{"with spaces", "john doe", false},
		{"with tab", "john\tdoe", false},
		{"whitespace trimmed", "  user  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateUsername(tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUsername() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result == "" && tt.username != "" {
				t.Error("ValidateUsername() returned empty for non-empty input")
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		wantErr   bool
		expectErr error
	}{
		{"valid password", "mypassword123", false, nil},
		{"valid complex", "P@ssw0rd!#$%", false, nil},
		{"empty password", "", true, ErrInvalidPassword},
		{"single char", "a", false, nil},
		{"max length", strings.Repeat("a", 4096), false, nil},
		{"too long", strings.Repeat("a", 4097), true, ErrPasswordTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != tt.expectErr {
				t.Errorf("ValidatePassword() error = %v, want %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidateNotes(t *testing.T) {
	tests := []struct {
		name  string
		notes string
		want  string
	}{
		{"empty notes", "", ""},
		{"normal notes", "My notes here", "My notes here"},
		{"with newline", "Line1\nLine2", "Line1\nLine2"},
		{"with tab", "Col1\tCol2", "Col1\tCol2"},
		{"with null char removed", "text\x00here", "texthere"},
		{"whitespace trimmed", "  trimmed  ", "trimmed"},
		{"control chars removed", "test\x01\x02end", "testend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateNotes(tt.notes)
			if err != nil {
				t.Errorf("ValidateNotes() error = %v", err)
			}
			if result != tt.want {
				t.Errorf("ValidateNotes() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestValidateDeviceName(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		wantErr    bool
	}{
		{"valid name", "My MacBook", false},
		{"valid simple", "Desktop", false},
		{"empty name", "", true},
		{"whitespace only", "   ", true},
		{"with numbers", "Device123", false},
		{"normalizes whitespace", "  Laptop  ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateDeviceName(tt.deviceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDeviceName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && result == "" && tt.deviceName != "" {
				t.Error("ValidateDeviceName() returned empty for non-empty input")
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		maxLen  int
		want    string
		wantErr bool
	}{
		{"normal string", "hello", 10, "hello", false},
		{"trims whitespace", "  hello  ", 10, "hello", false},
		{"too long", "this is a very long string", 10, "", true},
		{"removes control chars", "hel\x00lo", 10, "hello", false},
		{"keeps newlines", "line1\nline2", 20, "line1\nline2", false},
		{"keeps tabs", "col1\tcol2", 20, "col1\tcol2", false},
		{"removes other control", "test\x01\x02end", 20, "testend", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SanitizeString(tt.input, tt.maxLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("SanitizeString() error = %v, wantErr %v", err, tt.wantErr)
			}
			if result != tt.want {
				t.Errorf("SanitizeString() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestValidationConstants(t *testing.T) {
	if MaxSiteLength == 0 {
		t.Error("MaxSiteLength should not be 0")
	}
	if MaxUsernameLength == 0 {
		t.Error("MaxUsernameLength should not be 0")
	}
	if MaxPasswordLength == 0 {
		t.Error("MaxPasswordLength should not be 0")
	}
	if MaxNotesLength == 0 {
		t.Error("MaxNotesLength should not be 0")
	}
	if MaxDeviceName == 0 {
		t.Error("MaxDeviceName should not be 0")
	}
	if MinPasswordLength < 0 {
		t.Error("MinPasswordLength should not be negative")
	}
}
