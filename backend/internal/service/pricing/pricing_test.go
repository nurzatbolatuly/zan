package pricing_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/pricing"
)

// prices — тот же демо-набор, что frontend/src/features/tariffs/mocks.ts
// (SERVICE_PRICES) и frontend/src/features/settings/mocks.ts, чтобы
// golden-кейсы ниже были буквально теми же числами, что уже проверены
// юнит-тестами tariffPricing.test.ts на фронте (BACKEND_PLAN.md §5,
// Stage 2: "побитово совпадает с тем, что уже реализовано на фронте").
var prices = map[string]int{"qa": 2900, "doc": 4900}

func TestCompute(t *testing.T) {
	tests := []struct {
		name            string
		items           []domain.BundleItem
		discountPercent int
		wantSubtotal    int
		wantTotal       int
	}{
		{
			name:            "single item, no discount — b1",
			items:           []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
			discountPercent: 0,
			wantSubtotal:    2900,
			wantTotal:       2900,
		},
		{
			name:            "single item, 15% discount — b2",
			items:           []domain.BundleItem{{ServiceID: "qa", Qty: 3}},
			discountPercent: 15,
			wantSubtotal:    8700,
			wantTotal:       7400, // 8700*0.85 = 7395 -> round to nearest 10 = 7400
		},
		{
			name: "mixed items, 10% discount — b3",
			items: []domain.BundleItem{
				{ServiceID: "qa", Qty: 2},
				{ServiceID: "doc", Qty: 1},
			},
			discountPercent: 10,
			wantSubtotal:    10700,
			wantTotal:       9630, // 10700*0.9 = 9630, already multiple of 10
		},
		{
			name: "mixed items, 25% discount — b4",
			items: []domain.BundleItem{
				{ServiceID: "qa", Qty: 10},
				{ServiceID: "doc", Qty: 3},
			},
			discountPercent: 25,
			wantSubtotal:    43700,
			wantTotal:       32780, // 43700*0.75 = 32775 -> round to nearest 10 = 32780
		},
		{
			name: "custom order — no discount ever applied (v2 §4.1)",
			items: []domain.BundleItem{
				{ServiceID: "qa", Qty: 5},
				{ServiceID: "doc", Qty: 2},
			},
			discountPercent: 0,
			wantSubtotal:    24300,
			wantTotal:       24300,
		},
		{
			name:            "zero items",
			items:           nil,
			discountPercent: 20,
			wantSubtotal:    0,
			wantTotal:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pricing.Compute(tt.items, prices, tt.discountPercent)
			assert.Equal(t, tt.wantSubtotal, got.SubtotalTenge, "subtotal")
			assert.Equal(t, tt.wantTotal, got.TotalTenge, "total")
		})
	}
}
