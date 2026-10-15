package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// BankAccountCipher encrypts and decrypts bank account numbers with AES-256-GCM.
type BankAccountCipher struct {
	aead cipher.AEAD
}

// NewBankAccountCipher builds an AES-256-GCM cipher. The key must be exactly 32 bytes.
func NewBankAccountCipher(key []byte) (*BankAccountCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("bank account cipher key must be exactly 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}
	return &BankAccountCipher{aead: aead}, nil
}

// Encrypt seals one account number with a fresh random nonce and returns base64 ciphertext.
func (c *BankAccountCipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens base64 ciphertext produced by Encrypt.
func (c *BankAccountCipher) Decrypt(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("ciphertext is not valid base64: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext is shorter than the nonce")
	}
	plaintext, err := c.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", errors.New("failed to authenticate ciphertext")
	}
	return string(plaintext), nil
}

// MaskAccountNumber renders an account number for display, keeping the first and last four digits.
// Numbers of eight digits or fewer keep only the last four, because showing first-and-last of a
// short number would disclose the whole value.
func MaskAccountNumber(accountNumber string) string {
	digits := []rune(strings.TrimSpace(accountNumber))
	if len(digits) <= 4 {
		return string(digits)
	}
	if len(digits) <= 8 {
		masked := make([]rune, 0, len(digits))
		for range digits[:len(digits)-4] {
			masked = append(masked, 'x')
		}
		return string(append(masked, digits[len(digits)-4:]...))
	}
	masked := make([]rune, 0, len(digits))
	masked = append(masked, digits[:4]...)
	for range digits[4 : len(digits)-4] {
		masked = append(masked, 'x')
	}
	masked = append(masked, digits[len(digits)-4:]...)
	return string(masked)
}
