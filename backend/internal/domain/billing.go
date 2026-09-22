package domain

import (
	"fmt"
	"time"
)

// UserCredit — остаток сессии по одной услуге (уникален по
// session_id+service_id, списывается под SELECT ... FOR UPDATE —
// zan-backend-tz-v2.md §4.2).
type UserCredit struct {
	ServiceID string
	Quantity  int
}

// PaymentKind — что покупалось (zan-backend-tz-v2.md §2.5).
type PaymentKind string

const (
	PaymentKindSingleService PaymentKind = "single_service"
	PaymentKindTariff        PaymentKind = "tariff"
	PaymentKindCustom        PaymentKind = "custom"
)

// ParsePaymentKind валидирует сырое значение (из тела запроса) как PaymentKind.
func ParsePaymentKind(s string) (PaymentKind, error) {
	switch v := PaymentKind(s); v {
	case PaymentKindSingleService, PaymentKindTariff, PaymentKindCustom:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid payment kind %q", s)
	}
}

// PaymentStatus — статус мок-оплаты (zan-backend-tz-v2.md §2.5).
type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusSuccess PaymentStatus = "success"
	PaymentStatusFailed  PaymentStatus = "failed"
)

// ParsePaymentStatus валидирует сырое значение (из БД) как PaymentStatus.
func ParsePaymentStatus(s string) (PaymentStatus, error) {
	switch v := PaymentStatus(s); v {
	case PaymentStatusPending, PaymentStatusSuccess, PaymentStatusFailed:
		return v, nil
	default:
		return "", fmt.Errorf("domain: invalid payment status %q", s)
	}
}

// Payment — платёж, пополняющий баланс (zan-backend-tz-v2.md §2.5).
// Provider — всегда "mock" на этом этапе (§1: "заглушка", реальный шлюз —
// вне скоупа, backend-roadmap.md §7). ThreadID — заполняется, когда
// checkout вызван за конкретный тред (баланс кончился в момент открытия —
// v2 §4.2 п.2); использование этого поля для отметки thread.is_paid=true —
// задача Stage 3, здесь платёж только хранит связь.
type Payment struct {
	ID          string
	SessionID   string
	Kind        PaymentKind
	TariffID    *string
	ThreadID    *string
	Items       []BundleItem
	AmountTenge int
	Status      PaymentStatus
	Provider    string
	CreatedAt   time.Time
	PaidAt      *time.Time
}
	