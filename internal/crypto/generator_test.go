package crypto

import (
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	password, err := GeneratePassword(16)
	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	if len(password) != 16 {
		t.Errorf("Password length mismatch: got %d, want 16", len(password))
	}
}

func TestGenerateStrongPassword(t *testing.T) {
	password, err := GenerateStrongPassword(20)
	if err != nil {
		t.Fatalf("Failed to generate strong password: %v", err)
	}

	if len(password) != 20 {
		t.Errorf("Password length mismatch: got %d, want 20", len(password))
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, c := range password {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		case containsSpecial(c):
			hasSpecial = true
		}
	}

	if !hasLower {
		t.Error("Password should contain lowercase letters")
	}
	if !hasUpper {
		t.Error("Password should contain uppercase letters")
	}
	if !hasDigit {
		t.Error("Password should contain digits")
	}
	if !hasSpecial {
		t.Error("Password should contain special characters")
	}
}

func containsSpecial(c rune) bool {
	special := "!@#$%^&*()_+-=[]{}|;:,.<>?"
	for _, s := range special {
		if c == s {
			return true
		}
	}
	return false
}

func TestGeneratorOptions(t *testing.T) {
	g := NewGenerator(
		WithLength(30),
		WithLowercase(false),
		WithUppercase(true),
		WithDigits(false),
		WithSpecial(true),
	)

	if g.length != 30 {
		t.Errorf("Length mismatch: got %d, want 30", g.length)
	}

	if g.useLowercase {
		t.Error("Lowercase should be disabled")
	}

	if !g.useUppercase {
		t.Error("Uppercase should be enabled")
	}

	if g.useDigits {
		t.Error("Digits should be disabled")
	}

	if !g.useSpecial {
		t.Error("Special should be enabled")
	}
}

func TestGenerateWithMinLength(t *testing.T) {
	password, err := GenerateStrongPassword(4)
	if err != nil {
		t.Fatalf("Failed to generate password: %v", err)
	}

	if len(password) < 8 {
		t.Errorf("Password should be at least 8 chars, got %d", len(password))
	}
}
