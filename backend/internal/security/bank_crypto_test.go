package security_test

import (
	"strings"
	"testing"

	"github.com/galeriku/core/internal/security"
)

func TestBankAccountCipher_RoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes for AES-256
	cipher, err := security.NewBankAccountCipher(key)
	if err != nil {
		t.Fatalf("failed to build cipher: %v", err)
	}

	plaintext := "1234567890123456"
	encrypted, err := cipher.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("ciphertext must not equal plaintext")
	}
	if strings.Contains(encrypted, plaintext) {
		t.Fatal("ciphertext must not embed the plaintext account number")
	}

	decrypted, err := cipher.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestBankAccountCipher_NonceIsRandomPerEncryption(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, _ := security.NewBankAccountCipher(key)

	first, err := cipher.Encrypt("9876543210")
	if err != nil {
		t.Fatalf("first encrypt failed: %v", err)
	}
	second, err := cipher.Encrypt("9876543210")
	if err != nil {
		t.Fatalf("second encrypt failed: %v", err)
	}
	if first == second {
		t.Fatal("two encryptions of the same plaintext must differ (random nonce)")
	}
}

func TestBankAccountCipher_RejectsWrongKeyLength(t *testing.T) {
	for _, size := range []int{0, 16, 24, 31, 33} {
		if _, err := security.NewBankAccountCipher(make([]byte, size)); err == nil {
			t.Fatalf("expected rejection for %d-byte key, got nil", size)
		}
	}
}

func TestBankAccountCipher_RejectsTamperedCiphertext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	cipher, _ := security.NewBankAccountCipher(key)

	encrypted, err := cipher.Encrypt("5555444433332222")
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	tampered := []byte(encrypted)
	tampered[len(tampered)-1] ^= 0xFF
	if _, err := cipher.Decrypt(string(tampered)); err == nil {
		t.Fatal("expected authentication failure for tampered ciphertext, got nil")
	}
}

func TestMaskAccountNumber(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1234567890123456", "1234xxxxxxxx3456"},
		{"123456789012", "1234xxxx9012"},
		{"12345678", "xxxx5678"},
		{"1234", "1234"},
		{"123", "123"},
	}
	for _, c := range cases {
		if got := security.MaskAccountNumber(c.in); got != c.want {
			t.Errorf("MaskAccountNumber(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
