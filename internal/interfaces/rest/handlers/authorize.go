package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) AuthorizePayment(
	ctx context.Context,
	request api.AuthorizePaymentRequestObject,
) (api.AuthorizePaymentResponseObject, error) {

	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	cmd := services.AuthorizeCommand{
		OrderID:     req.OrderId,
		CustomerID:  req.CustomerId,
		Amount:      req.Amount,
		Currency:    "USD",
		CardNumber:  req.CardNumber,
		CVV:         req.Cvv,
		ExpiryMonth: req.ExpiryMonth,
		ExpiryYear:  req.ExpiryYear,
	}

	payment, err := h.authService.Authorize(ctx, cmd, idempotencyKey)
	if err != nil {
		return mapAuthServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapAuthServiceErrorToAPIResponse(ctx, err)
	}

	return api.AuthorizePayment201JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapAuthServiceErrorToAPIResponse(ctx context.Context, err error) (api.AuthorizePaymentResponseObject, error) {
	statusCode := application.ToHTTPStatus(err)
	errorCode := application.ToErrorCode(err)

	errorResponse := api.ErrorResponse{
		Success: false,
		Error: struct {
			Code    api.ErrorResponseErrorCode `json:"code"`
			Message string                     `json:"message"`
		}{
			Code:    api.ErrorResponseErrorCode(errorCode),
			Message: err.Error(),
		},
	}

	switch statusCode {
	case http.StatusBadRequest:
		return api.AuthorizePayment400JSONResponse(errorResponse), nil

	case http.StatusRequestTimeout:
		return api.AuthorizePayment408JSONResponse(errorResponse), nil

	case http.StatusConflict:
		return api.AuthorizePayment409JSONResponse(errorResponse), nil

	case http.StatusInternalServerError:
		return api.AuthorizePayment500JSONResponse(errorResponse), nil

	default:
		return api.AuthorizePayment500JSONResponse(errorResponse), nil
	}
}
