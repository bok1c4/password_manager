package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const (
	lowercase = "abcdefghijklmnopqrstuvwxyz"
	uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digits    = "0123456789"
	special   = "!@#$%^&*()_+-=[]{}|;:,.<>?"
	allChars  = lowercase + uppercase + digits + special
)

type Generator struct {
	length       int
	useLowercase bool
	useUppercase bool
	useDigits    bool
	useSpecial   bool
}

type GeneratorOption func(*Generator)

func WithLength(length int) GeneratorOption {
	return func(g *Generator) {
		g.length = length
	}
}

func WithLowercase(enabled bool) GeneratorOption {
	return func(g *Generator) {
		g.useLowercase = enabled
	}
}

func WithUppercase(enabled bool) GeneratorOption {
	return func(g *Generator) {
		g.useUppercase = enabled
	}
}

func WithDigits(enabled bool) GeneratorOption {
	return func(g *Generator) {
		g.useDigits = enabled
	}
}

func WithSpecial(enabled bool) GeneratorOption {
	return func(g *Generator) {
		g.useSpecial = enabled
	}
}

func NewGenerator(opts ...GeneratorOption) *Generator {
	g := &Generator{
		length:       20,
		useLowercase: true,
		useUppercase: true,
		useDigits:    true,
		useSpecial:   true,
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func (g *Generator) charset() string {
	charset := ""
	if g.useLowercase {
		charset += lowercase
	}
	if g.useUppercase {
		charset += uppercase
	}
	if g.useDigits {
		charset += digits
	}
	if g.useSpecial {
		charset += special
	}
	if charset == "" {
		charset = allChars
	}
	return charset
}

func (g *Generator) Generate() (string, error) {
	charset := g.charset()
	result := make([]byte, g.length)

	for i := 0; i < g.length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		result[i] = charset[n.Int64()]
	}

	return string(result), nil
}

func GeneratePassword(length int) (string, error) {
	return NewGenerator(WithLength(length)).Generate()
}

func GenerateStrongPassword(length int) (string, error) {
	if length < 8 {
		length = 8
	}
	g := NewGenerator(
		WithLength(length),
		WithLowercase(true),
		WithUppercase(true),
		WithDigits(true),
		WithSpecial(true),
	)
	return g.Generate()
}
