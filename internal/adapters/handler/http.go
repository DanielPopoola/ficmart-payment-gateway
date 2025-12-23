package handler

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/go-playground/validator"
	"github.com/google/uuid"
)

type AuthorizationService interface {
	Authorize(ctx context.Context, orderID, customerID, idempotencyKey string, amount int64, cardNumber, cvv string, expiryMonth, expiryYear int) (*domain.Payment, error)
}

type CaptureService interface {
	Capture(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
}

type RefundService interface {
	Refund(ctx context.Context, paymentID uuid.UUID, amount int64, idempotencyKey string) (*domain.Payment, error)
}

type VoidService interface {
	Void(ctx context.Context, paymentID uuid.UUID, idempotencyKey string) (*domain.Payment, error)
}

type PaymentHandler struct {
	authService    AuthorizationService
	captureService CaptureService
	refundService  RefundService
	voidService    VoidService
	validate       *validator.Validate
}

func NewPaymentHandler(
	authService AuthorizationService,
	captureService CaptureService,
	refundService RefundService,
	voidService VoidService,
) *PaymentHandler {
	return &PaymentHandler{
		authService:    authService,
		captureService: captureService,
		refundService:  refundService,
		voidService:    voidService,
		validate:       validator.New(),
	}
}

func (h *PaymentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /authorize", h.HandleAuthorize)
	mux.HandleFunc("POST /capture", h.HandleCapture)
	mux.HandleFunc("POST /refund", h.HandleRefund)
	mux.HandleFunc("POST /void", h.HandleVoid)
}
