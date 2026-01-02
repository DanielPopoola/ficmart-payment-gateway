package handlers

import (
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
}

func NewHandlers(
	authService *services.AuthorizeService,
	captureService *services.CaptureService,
	voidService *services.VoidService,
	refundService *services.RefundService,
	queryService *services.QueryService,
) *Handlers {
	return &Handlers{
		authService:    authService,
		captureService: captureService,
		voidService:    voidService,
		refundService:  refundService,
		queryService:   queryService,
	}
}

// Ensure Handlers implements StrictServerInterface
var _ api.StrictServerInterface = (*Handlers)(nil)
