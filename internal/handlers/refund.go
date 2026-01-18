package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
)

func (h *Handlers) RefundPayment(
	ctx context.Context,
	request api.RefundPaymentRequestObject,
) (api.RefundPaymentResponseObject, error) {
	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	paymentID := req.PaymentId.String()
	payment, err := h.refundService.Refund(ctx, paymentID, idempotencyKey)
	if err != nil {
		return mapRefundServiceErrorToAPIResponse(err)
	}

	apiPayment, err := ToAPIPayment(payment)
	if err != nil {
		return mapRefundServiceErrorToAPIResponse(err)
	}

	return api.RefundPayment200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapRefundServiceErrorToAPIResponse(err error) (api.RefundPaymentResponseObject, error) {
	statusCode, errorResponse := BuildErrorResponse(err)

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
