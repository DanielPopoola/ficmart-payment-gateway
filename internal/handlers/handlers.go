package handlers

import (
	"log/slog"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/service"
)

type Handler struct {
	authService    service.Authorizer
	captureService service.Capturer
	voidService    service.Voider
	refundService  service.Refunder
	queryService   service.PaymentQuery
	logger         *slog.Logger
}

func NewHandler(
	authService service.Authorizer,
	captureService service.Capturer,
	voidService service.Voider,
	refundService service.Refunder,
	queryService service.PaymentQuery,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		authService:    authService,
		captureService: captureService,
		voidService:    voidService,
		refundService:  refundService,
		queryService:   queryService,
		logger:         logger,
	}
}
