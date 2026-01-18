package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) CapturePayment(
	ctx context.Context,
	request api.CapturePaymentRequestObject,
) (api.CapturePaymentResponseObject, error) {
	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	paymentID := req.PaymentId.String()
	payment, err := h.captureService.Capture(ctx, paymentID, idempotencyKey)
	if err != nil {
		return mapCaptureServiceErrorToAPIResponse(err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapCaptureServiceErrorToAPIResponse(err)
	}

	return api.CapturePayment200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapCaptureServiceErrorToAPIResponse(err error) (api.CapturePaymentResponseObject, error) {
	statusCode, errorResponse := rest.BuildErrorResponse(err)

	switch statusCode {
	case http.StatusBadRequest:
		return api.CapturePayment400JSONResponse(errorResponse), nil
	case http.StatusNotFound:
		return api.CapturePayment404JSONResponse(errorResponse), nil
	case http.StatusRequestTimeout:
		return api.CapturePayment408JSONResponse(errorResponse), nil
	case http.StatusConflict:
		return api.CapturePayment409JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.CapturePayment500JSONResponse(errorResponse), nil
	default:
		return api.CapturePayment500JSONResponse(errorResponse), nil
	}
}
