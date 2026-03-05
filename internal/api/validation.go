package api

import (
	"errors"
	"strings"
)

const (
	MaxSiteLength     = 256
	MaxUsernameLength = 256
	MaxPasswordLength = 4096
	MaxNotesLength    = 10000
	MaxDeviceName     = 100
	MinPasswordLength = 1
)

var (
	ErrInvalidSite     = errors.New("invalid site")
	ErrInvalidUsername = errors.New("invalid username")
	ErrInvalidPassword = errors.New("invalid password")
	ErrInvalidNotes    = errors.New("invalid notes")
	ErrSiteTooLong     = errors.New("site name too long")
	ErrUsernameTooLong = errors.New("username too long")
	ErrPasswordTooLong = errors.New("password too long")
	ErrNotesTooLong    = errors.New("notes too long")
)

func SanitizeString(s string, maxLen int) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return "", errors.New("string too long")
	}
	sanitized := strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	return sanitized, nil
}

func ValidateSite(site string) (string, error) {
	if site == "" {
		return "", ErrInvalidSite
	}
	sanitized, err := SanitizeString(site, MaxSiteLength)
	if err != nil {
		return "", err
	}
	if sanitized == "" {
		return "", ErrInvalidSite
	}
	return sanitized, nil
}

func ValidateUsername(username string) (string, error) {
	return SanitizeString(username, MaxUsernameLength)
}

func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrInvalidPassword
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}
	return nil
}

func ValidateNotes(notes string) (string, error) {
	return SanitizeString(notes, MaxNotesLength)
}

func ValidateDeviceName(name string) (string, error) {
	if name == "" {
		return "", errors.New("device name required")
	}
	sanitized, err := SanitizeString(name, MaxDeviceName)
	if err != nil {
		return "", err
	}
	if sanitized == "" {
		return "", errors.New("device name required")
	}
	return sanitized, nil
}
