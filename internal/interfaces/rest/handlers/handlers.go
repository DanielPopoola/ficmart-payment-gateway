package handlers

import (
	"log/slog"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
)

// Handlers implements the OpenAPI StrictServerInterface
type Handlers struct {
	authService    *services.AuthorizeService
	captureService *services.CaptureService
	voidService    *services.VoidService
	refundService  *services.RefundService
	queryService   *services.QueryService
	logger         *slog.Logger
}

func NewHandlers(
	authService *services.AuthorizeService,
	captureService *services.CaptureService,
	voidService *services.VoidService,
	refundService *services.RefundService,
	queryService *services.QueryService,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		authService:    authService,
		captureService: captureService,
		voidService:    voidService,
		refundService:  refundService,
		queryService:   queryService,
		logger:         logger,
	}
}

// Ensure Handlers implements StrictServerInterface
var _ api.StrictServerInterface = (*Handlers)(nil)
