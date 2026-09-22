package domain

import "time"

// Service — предоставляемая услуга, базовая единица тарификации
// (zan-backend-tz-v2.md §2.2). ID — slug (`qa`/`doc`), не UUID: услуги
// заводятся миграцией (backend/migrations/000003_stage2_billing.up.sql),
// не через API — v2 §3.7 даёт /admin/services только GET/PUT, без POST.
// PriceTenge — целые тенге (BACKEND_PLAN.md §6 п.7), не тиыны и не decimal.
type Service struct {
	ID         string
	TypeLabel  string
	Name       string
	PriceTenge int
	IsActive   bool
	UpdatedAt  time.Time
}

// BundleItem — одна позиция набора услуг: сколько единиц ServiceID входит
// в Tariff (zan-backend-tz-v2.md §2.3) или в разовую покупку/custom-набор
// (§4.1). Общий тип для Tariff.Items и Payment.Items.
type BundleItem struct {
	ServiceID string
	Qty       int
}

// Tariff — фиксированный бандл услуг со скидкой (zan-backend-tz-v2.md
// §2.3). Цена не хранится на самой сущности — считается на лету по
// текущим ценам Service (internal/service/pricing), чтобы не расходиться
// при изменении цены услуги (§4.1).
type Tariff struct {
	ID              string
	Name            string
	DiscountPercent int
	Items           []BundleItem
	IsActive        bool
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
