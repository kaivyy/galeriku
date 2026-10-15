package domain

import (
	"time"

	"github.com/google/uuid"
)

// User merepresentasikan identitas global pengguna Galeriku
type User struct {
	ID              uuid.UUID `json:"id"`
	EmailNormalized *string   `json:"email_normalized,omitempty"`
	PhoneE164       *string   `json:"phone_e164,omitempty"`
	AccountStatus   string    `json:"account_status"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Order merepresentasikan pesanan pembelian foto
type Order struct {
	ID             uuid.UUID `json:"id"`
	OrderNumber    string    `json:"order_number"`
	BuyerUserID    uuid.UUID `json:"buyer_user_id"`
	Status         string    `json:"status"`
	TotalAmountIDR int64     `json:"total_amount_idr"` // Integer Rupiah Bulat (BIGINT)
	Currency       string    `json:"currency"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Photo merepresentasikan entitas foto logis dalam FotoTree
type Photo struct {
	ID               uuid.UUID  `json:"id"`
	UploaderUserID   uuid.UUID  `json:"uploader_user_id"`
	Status           string     `json:"status"`
	OriginalFilename string     `json:"original_filename"`
	TakenAt          *time.Time `json:"taken_at,omitempty"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	CreatedAt        time.Time  `json:"created_at"`
}
