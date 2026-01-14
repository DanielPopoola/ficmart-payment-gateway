package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) VoidPayment(
	ctx context.Context,
	request api.VoidPaymentRequestObject,
) (api.VoidPaymentResponseObject, error) {
	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	cmd := services.VoidCommand{
		PaymentID: req.PaymentId.String(),
	}

	payment, err := h.voidService.Void(ctx, cmd, idempotencyKey)
	if err != nil {
		return mapVoidServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapVoidServiceErrorToAPIResponse(ctx, err)
	}

	return api.VoidPayment200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapVoidServiceErrorToAPIResponse(ctx context.Context, err error) (api.VoidPaymentResponseObject, error) {
	statusCode, errorResponse := rest.BuildErrorResponse(err)

	switch statusCode {
	case http.StatusBadRequest:
		return api.VoidPayment400JSONResponse(errorResponse), nil
	case http.StatusNotFound:
		return api.VoidPayment404JSONResponse(errorResponse), nil
	case http.StatusRequestTimeout:
		return api.VoidPayment408JSONResponse(errorResponse), nil
	case http.StatusConflict:
		return api.VoidPayment409JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.VoidPayment500JSONResponse(errorResponse), nil
	default:
		return api.VoidPayment500JSONResponse(errorResponse), nil
	}
}
