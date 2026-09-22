package httpserver

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"zan-backend/internal/apierror"
	"zan-backend/internal/domain"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/service/catalog"
)

// serviceResponse — {id, type_label, name, price, is_active, updated_at}
// (zan-backend-tz-v2.md §2.2). price — целые тенге (BACKEND_PLAN.md §6 п.7).
type serviceResponse struct {
	ID        string `json:"id"`
	TypeLabel string `json:"type_label"`
	Name      string `json:"name"`
	Price     int    `json:"price"`
	IsActive  bool   `json:"is_active"`
}

func newServiceResponse(s domain.Service) serviceResponse {
	return serviceResponse{
		ID:        s.ID,
		TypeLabel: s.TypeLabel,
		Name:      s.Name,
		Price:     s.PriceTenge,
		IsActive:  s.IsActive,
	}
}

// bundleItemResponse — {service_id, qty} (zan-backend-tz-v2.md §2.3).
type bundleItemResponse struct {
	ServiceID string `json:"service_id"`
	Qty       int    `json:"qty"`
}

func newBundleItemResponses(items []domain.BundleItem) []bundleItemResponse {
	out := make([]bundleItemResponse, 0, len(items))
	for _, it := range items {
		out = append(out, bundleItemResponse{ServiceID: it.ServiceID, Qty: it.Qty})
	}
	return out
}

// tariffResponse — тариф с рассчитанной ценой (zan-backend-tz-v2.md §2.3,
// §3.4: "активные бандлы с рассчитанной ценой: subtotal, total, discount_percent").
type tariffResponse struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	DiscountPercent int                  `json:"discount_percent"`
	Items           []bundleItemResponse `json:"items"`
	Subtotal        int                  `json:"subtotal"`
	Total           int                  `json:"total"`
	IsActive        bool                 `json:"is_active"`
}

func newTariffResponse(tp catalog.TariffPrice) tariffResponse {
	return tariffResponse{
		ID:              tp.Tariff.ID,
		Name:            tp.Tariff.Name,
		DiscountPercent: tp.Tariff.DiscountPercent,
		Items:           newBundleItemResponses(tp.Tariff.Items),
		Subtotal:        tp.SubtotalTenge,
		Total:           tp.TotalTenge,
		IsActive:        tp.Tariff.IsActive,
	}
}

// listServicesHandler — GET /services.
func listServicesHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		services, err := svc.ListActiveServices(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("list_services_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]serviceResponse, 0, len(services))
		for _, s := range services {
			out = append(out, newServiceResponse(s))
		}
		c.JSON(http.StatusOK, out)
	}
}

// listTariffsHandler — GET /tariffs.
func listTariffsHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tariffs, err := svc.ListActiveTariffs(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("list_tariffs_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]tariffResponse, 0, len(tariffs))
		for _, tp := range tariffs {
			out = append(out, newTariffResponse(tp))
		}
		c.JSON(http.StatusOK, out)
	}
}

// adminListServicesHandler — GET /admin/services: все услуги, включая неактивные.
func adminListServicesHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		services, err := svc.ListAllServices(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("admin_list_services_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]serviceResponse, 0, len(services))
		for _, s := range services {
			out = append(out, newServiceResponse(s))
		}
		c.JSON(http.StatusOK, out)
	}
}

// updateServiceRequest — PUT /admin/services/{id}: price/is_active
// (zan-backend-tz-v2.md §3.7) — оба поля обязательны (полная замена
// мутируемых полей, не частичный PATCH).
type updateServiceRequest struct {
	Price    *int  `json:"price" binding:"required"`
	IsActive *bool `json:"is_active" binding:"required"`
}

// adminUpdateServiceHandler — PUT /admin/services/{id}.
func adminUpdateServiceHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateServiceRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Price == nil || req.IsActive == nil {
			writeError(c, invalidRequestError("price and is_active are required"))
			return
		}

		updated, err := svc.UpdateServicePricing(c.Request.Context(), c.Param("id"), *req.Price, *req.IsActive)
		if err != nil {
			if errors.Is(err, catalog.ErrServiceNotFound) {
				writeError(c, notFoundError("service_not_found", "Service not found"))
				return
			}
			if errors.Is(err, catalog.ErrInvalidItems) {
				writeError(c, invalidRequestError(err.Error()))
				return
			}
			logger.FromContext(c.Request.Context()).Error("admin_update_service_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.JSON(http.StatusOK, newServiceResponse(updated))
	}
}

// adminListTariffsHandler — GET /admin/tariffs: все бандлы, включая неактивные.
func adminListTariffsHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		tariffs, err := svc.ListAllTariffs(c.Request.Context())
		if err != nil {
			logger.FromContext(c.Request.Context()).Error("admin_list_tariffs_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		out := make([]tariffResponse, 0, len(tariffs))
		for _, tp := range tariffs {
			out = append(out, newTariffResponse(tp))
		}
		c.JSON(http.StatusOK, out)
	}
}

// bundleItemRequest — {service_id, qty} на входе (checkout/tariff write).
type bundleItemRequest struct {
	ServiceID string `json:"service_id" binding:"required"`
	Qty       int    `json:"qty" binding:"required"`
}

// tariffWriteRequest — тело POST/PUT /admin/tariffs(/{id}): {name,
// discount_percent, items} (zan-backend-tz-v2.md §3.7).
type tariffWriteRequest struct {
	Name            string              `json:"name" binding:"required"`
	DiscountPercent int                 `json:"discount_percent"`
	Items           []bundleItemRequest `json:"items" binding:"required"`
}

func (r tariffWriteRequest) toDomainItems() []domain.BundleItem {
	items := make([]domain.BundleItem, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, domain.BundleItem{ServiceID: it.ServiceID, Qty: it.Qty})
	}
	return items
}

// adminCreateTariffHandler — POST /admin/tariffs.
func adminCreateTariffHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req tariffWriteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}

		created, err := svc.CreateTariff(c.Request.Context(), req.Name, req.DiscountPercent, req.toDomainItems())
		if err != nil {
			writeTariffValidationError(c, err)
			return
		}
		c.JSON(http.StatusCreated, newTariffResponse(catalog.TariffPrice{Tariff: created}))
	}
}

// adminUpdateTariffHandler — PUT /admin/tariffs/{id}.
func adminUpdateTariffHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req tariffWriteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeError(c, invalidRequestError("Invalid request body"))
			return
		}

		updated, err := svc.UpdateTariff(c.Request.Context(), c.Param("id"), req.Name, req.DiscountPercent, req.toDomainItems())
		if err != nil {
			writeTariffValidationError(c, err)
			return
		}
		c.JSON(http.StatusOK, newTariffResponse(catalog.TariffPrice{Tariff: updated}))
	}
}

// adminDeactivateTariffHandler — DELETE /admin/tariffs/{id}: is_active=false
// (архив, история платежей не трогается — zan-backend-tz-v2.md §3.7).
func adminDeactivateTariffHandler(svc *catalog.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, err := svc.DeactivateTariff(c.Request.Context(), c.Param("id"))
		if err != nil {
			if errors.Is(err, catalog.ErrTariffNotFound) {
				writeError(c, notFoundError("tariff_not_found", "Tariff not found"))
				return
			}
			logger.FromContext(c.Request.Context()).Error("admin_deactivate_tariff_failed",
				slog.Group("context", slog.String("error", err.Error())))
			writeError(c, apierror.Internal())
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func writeTariffValidationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalidItems), errors.Is(err, catalog.ErrInvalidDiscount):
		writeError(c, invalidRequestError(err.Error()))
	case errors.Is(err, catalog.ErrTariffNotFound):
		writeError(c, notFoundError("tariff_not_found", "Tariff not found"))
	default:
		logger.FromContext(c.Request.Context()).Error("admin_tariff_write_failed",
			slog.Group("context", slog.String("error", err.Error())))
		writeError(c, apierror.Internal())
	}
}
