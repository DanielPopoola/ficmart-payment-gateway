package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) RefundPayment(
	ctx context.Context,
	request api.RefundPaymentRequestObject,
) (api.RefundPaymentResponseObject, error) {
	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	cmd := services.RefundCommand{
		PaymentID: req.PaymentId.String(),
		Amount:    req.Amount,
	}

	payment, err := h.refundService.Refund(ctx, cmd, idempotencyKey)
	if err != nil {
		return mapRefundServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapRefundServiceErrorToAPIResponse(ctx, err)
	}

	return api.RefundPayment200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapRefundServiceErrorToAPIResponse(ctx context.Context, err error) (api.RefundPaymentResponseObject, error) {
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
		return api.RefundPayment400JSONResponse(errorResponse), nil
	case http.StatusNotFound:
		return api.RefundPayment404JSONResponse(errorResponse), nil
	case http.StatusRequestTimeout:
		return api.RefundPayment408JSONResponse(errorResponse), nil
	case http.StatusConflict:
		return api.RefundPayment409JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.RefundPayment500JSONResponse(errorResponse), nil
	default:
		return api.RefundPayment500JSONResponse(errorResponse), nil
	}
}
