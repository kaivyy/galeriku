package handler_test

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"

	"github.com/galeriku/core/internal/domain"
)

// computeStubSignature mirrors usecase.StubPaymentGateway.VerifyWebhookSignature.
// The stub computes HMAC-SHA512 over order_id+status+amount+serverKey.
func computeStubSignature(serverKey string, event domain.WebhookEvent) string {
	raw := event.OrderID + event.TransactionStatus + event.GrossAmount + serverKey
	mac := hmac.New(sha512.New, []byte(serverKey))
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}
