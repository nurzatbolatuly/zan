// Package pricing — единственная реализация формулы цены бандла
// (zan-backend-tz-v2.md §2.3), обязанная побитово совпадать со
// shared/lib/tariffPricing.ts на фронте (BACKEND_PLAN.md §5, Stage 2).
// Чистая функция без внешних зависимостей (кроме domain — который сам
// не тянет ничего, кроме stdlib) — не ходит в БД, не знает про
// domain.Tariff/domain.Service целиком, только про то, что нужно для
// расчёта (id услуги, её цена, qty).
package pricing

import (
	"math"

	"zan-backend/internal/domain"
)

// Bundle — результат расчёта: Subtotal — сумма price×qty без скидки,
// Total — итог к оплате.
type Bundle struct {
	SubtotalTenge int
	TotalTenge    int
}

// Compute считает цену набора items по текущим ценам prices (map
// serviceID -> price за 1 единицу) со скидкой discountPercent (0 — без
// скидки, custom-набор из v2 §4.1). Отсутствующая в prices услуга
// считается как price=0 — вызывающий код (internal/service/catalog,
// internal/service/billing) обязан провалидировать, что все serviceID из
// items существуют, до вызова Compute; здесь эта проверка не дублируется.
//
// Округление — ровно как frontend/src/shared/lib/tariffPricing.ts:
// округление до кратных 10 тенге применяется только когда скидка > 0.
// Без скидки Total == Subtotal без изменений — умышленно, чтобы не
// округлять единичные покупки без скидки, у которых Subtotal и так
// точная сумма целых цен.
func Compute(items []domain.BundleItem, prices map[string]int, discountPercent int) Bundle {
	subtotal := 0
	for _, item := range items {
		subtotal += prices[item.ServiceID] * item.Qty
	}

	if discountPercent <= 0 {
		return Bundle{SubtotalTenge: subtotal, TotalTenge: subtotal}
	}

	discounted := float64(subtotal) * (1 - float64(discountPercent)/100)
	return Bundle{SubtotalTenge: subtotal, TotalTenge: roundToTens(discounted)}
}

func roundToTens(v float64) int {
	return int(math.Round(v/10)) * 10
}
