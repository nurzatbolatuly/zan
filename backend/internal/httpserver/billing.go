package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/thread"
)

// balanceEntryResponse — {service_id, quantity} (zan-backend-tz-v2.md §3.4).
type balanceEntryResponse struct {
	ServiceID string `json:"service_id"`
	Quantity  int    `json:"quantity"`
}

// getBalanceHandler — GET /balance. Требует SessionAuth в группе маршрутов.
func getBalanceHandler(svc *billing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		credits, err := svc.GetBalance(c.Request.Context(), sess.ID)
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("get_balance_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]balanceEntryResponse, 0, len(credits))
		for _, cr := range credits {
			out = append(out, balanceEntryResponse{ServiceID: cr.ServiceID, Quantity: cr.Quantity})
		}
		c.JSON(http.StatusOK, out)
	}
}

// checkoutRequest — POST /payments/checkout (zan-backend-tz-v2.md §3.5).
type checkoutRequest struct {
	Kind     string              `json:"kind" binding:"required"`
	TariffID *string             `json:"tariff_id"`
	Items    []bundleItemRequest `json:"items"`
	ThreadID *string             `json:"thread_id"`
}

// checkoutResponse — {payment_id, amount} (zan-backend-tz-v2.md §3.5).
type checkoutResponse struct {
	PaymentID string `json:"payment_id"`
	Amount    int    `json:"amount"`
}

// checkoutHandler — POST /payments/checkout. Требует SessionAuth.
func checkoutHandler(svc *billing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req checkoutRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}

		kind, err := domain.ParsePaymentKind(req.Kind)
		if err != nil {
			writeError(c, invalidRequestError("Invalid kind"))
			return
		}

		items := make([]domain.BundleItem, 0, len(req.Items))
		for _, it := range req.Items {
			items = append(items, domain.BundleItem{ServiceID: it.ServiceID, Qty: it.Qty})
		}

		sess := sessionFromGin(c)
		payment, err := svc.Checkout(c.Request.Context(), billing.CheckoutRequest{
			SessionID: sess.ID,
			Kind:      kind,
			TariffID:  req.TariffID,
			Items:     items,
			ThreadID:  req.ThreadID,
		})
		if err != nil {
			writeCheckoutError(c, err)
			return
		}
		c.JSON(http.StatusCreated, checkoutResponse{PaymentID: payment.ID, Amount: payment.AmountTenge})
	}
}

// paymentResponse — GET /payments/{id}, POST /payments/{id}/confirm.
type paymentResponse struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	Amount   int     `json:"amount"`
	Status   string  `json:"status"`
	Provider string  `json:"provider"`
	PaidAt   *string `json:"paid_at,omitempty"`
}

func newPaymentResponse(p domain.Payment) paymentResponse {
	resp := paymentResponse{
		ID:       p.ID,
		Kind:     string(p.Kind),
		Amount:   p.AmountTenge,
		Status:   string(p.Status),
		Provider: p.Provider,
	}
	if p.PaidAt != nil {
		paidAt := p.PaidAt.Format(rfc3339Format)
		resp.PaidAt = &paidAt
	}
	return resp
}

// getPaymentHandler — GET /payments/{id}. Требует SessionAuth.
func getPaymentHandler(svc *billing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		p, err := svc.GetPayment(c.Request.Context(), c.Param("id"), sess.ID)
		if err != nil {
			if errors.Is(err, billing.ErrPaymentNotFound) {
				writeError(c, notFoundError("payment_not_found", "Payment not found"))
				return
			}
			logger.FromContext(c.Request.Context()).Error("get_payment_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.JSON(http.StatusOK, newPaymentResponse(p))
	}
}

// confirmPaymentHandler — POST /payments/{id}/confirm. Требует SessionAuth.
// Идемпотентно (zan-backend-tz-v3.md §5.1). threadSvc — Stage 3
// (zan-backend-tz-v2.md §4.2 п.2, BACKEND_LOG.md "Открыто для Stage 3+"):
// когда платёж привязан к треду (checkout {..., thread_id}), успешный
// confirm сразу активирует тред (списание уже произошло billing'ом выше —
// thread.ActivateAfterPayment лишь проставляет is_paid/free_until и
// синхронно запускает обработку, "купил и тут же потратил").
func confirmPaymentHandler(svc *billing.Service, threadSvc *thread.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessionFromGin(c)
		p, err := svc.ConfirmPayment(c.Request.Context(), c.Param("id"), sess.ID)
		if err != nil {
			if errors.Is(err, billing.ErrPaymentNotFound) {
				writeError(c, notFoundError("payment_not_found", "Payment not found"))
				return
			}
			if errors.Is(err, billing.ErrPaymentNotPending) {
				writeError(c, &apierror.Error{
					Code:       "payment_not_pending",
					Message:    "Оплата не прошла. Попробовать снова",
					HTTPStatus: http.StatusConflict,
				})
				return
			}
			logger.FromContext(c.Request.Context()).Error("confirm_payment_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}

		if p.Status == domain.PaymentStatusSuccess && p.ThreadID != nil {
			if _, err := threadSvc.ActivateAfterPayment(c.Request.Context(), *p.ThreadID, sess.ID, sess.Language); err != nil {
				// Платёж уже подтверждён и это отдаётся клиенту как успех —
				// тред просто не стартовал синхронно вместе с ним. Клиент
				// увидит его через GET /threads/{id} (polling,
				// BACKEND_PLAN.md §6 п.12) и может расследовать по
				// trace_id/thread_id в логах, если статус не сдвинется.
				logger.FromContext(c.Request.Context()).Error("activate_thread_after_payment_failed",
					slog.Group("context",
						slog.String("thread_id", *p.ThreadID),
						slog.String("error", err.Error()),
					))
			}
		}

		c.JSON(http.StatusOK, newPaymentResponse(p))
	}
}

func writeCheckoutError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrInvalidCheckout),
		errors.Is(err, catalog.ErrInvalidItems):
		writeError(c, invalidRequestError(err.Error()))
	case errors.Is(err, catalog.ErrTariffNotFound):
		writeError(c, notFoundError("tariff_not_found", "Tariff not found"))
	case errors.Is(err, billing.ErrTariffInactive):
		writeError(c, invalidRequestError("Tariff is not active"))
	default:
		logger.FromContext(c.Request.Context()).Error("checkout_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
