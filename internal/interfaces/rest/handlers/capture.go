package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) CapturePayment(
	ctx context.Context,
	request api.CapturePaymentRequestObject,
) (api.CapturePaymentResponseObject, error) {
	req := request.Body
	idempotencyKey := request.Params.IdempotencyKey

	cmd := services.CaptureCommand{
		PaymentID: req.PaymentId.String(),
		Amount:    req.Amount,
	}

	payment, err := h.captureService.Capture(ctx, cmd, idempotencyKey)
	if err != nil {
		return mapCaptureServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapCaptureServiceErrorToAPIResponse(ctx, err)
	}

	return api.CapturePayment200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapCaptureServiceErrorToAPIResponse(ctx context.Context, err error) (api.CapturePaymentResponseObject, error) {
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
