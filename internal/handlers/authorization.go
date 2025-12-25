package handlers

import (
	"context"
	"errors"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

func (h *Handler) AuthorizePayment(
	ctx context.Context,
	request api.AuthorizePaymentRequestObject,
) (api.AuthorizePaymentResponseObject, error) {
	txn, err := h.authService.Authorize(
		ctx,
		request.Body.OrderId, request.Body.CustomerId, request.Params.IdempotencyKey,
		request.Body.Amount, request.Body.CardNumber, request.Body.Cvv,
		request.Body.ExpiryMonth, request.Body.ExpiryYear,
	)

	if err != nil {
		return h.handleAuthorizationError(err)
	}

	return api.AuthorizePayment201JSONResponse{
		Success: true,
		Data:    mapDomainPaymentToAPI(txn),
	}, nil
}

// handleAuthorizationError maps service errors to appropriate HTTP responses
func (h *Handler) handleAuthorizationError(err error) (api.AuthorizePaymentResponseObject, error) {
	var domainErr *domain.DomainError
	if !errors.As(err, &domainErr) {
		return api.AuthorizePayment500JSONResponse{
			Success: false,
			Error: struct {
				Code    api.ErrorResponseErrorCode `json:"code"`
				Message string                     `json:"message"`
			}{Code: api.INTERNALERROR, Message: "Internal server error"},
		}, nil
	}

	switch domainErr.Code {
	case domain.ErrCodeDuplicateIdempotencyKey:
		return api.AuthorizePayment409JSONResponse{
			Success: false,
			Error: struct {
				Code    api.ErrorResponseErrorCode `json:"code"`
				Message string                     `json:"message"`
			}{Code: api.DUPLICATEIDEMPOTENCYKEY, Message: domainErr.Message},
		}, nil

	case domain.ErrRequestProcessing:
		// Return 202 Accepted if the request is still being processed
		return api.AuthorizePayment202JSONResponse{
			Success: false,
			Error: struct {
				Code    api.ErrorResponseErrorCode `json:"code"`
				Message string                     `json:"message"`
			}{Code: api.REQUESTPROCESSING, Message: domainErr.Message},
		}, nil

	case domain.ErrCodeInvalidAmount, domain.ErrCodeMissingRequiredField:
		return api.AuthorizePayment400JSONResponse{
			Success: false,
			Error: struct {
				Code    api.ErrorResponseErrorCode `json:"code"`
				Message string                     `json:"message"`
			}{Code: api.VALIDATIONERROR, Message: domainErr.Message},
		}, nil

	default:
		return api.AuthorizePayment500JSONResponse{
			Success: false,
			Error: struct {
				Code    api.ErrorResponseErrorCode `json:"code"`
				Message string                     `json:"message"`
			}{Code: api.INTERNALERROR, Message: domainErr.Message},
		}, nil
	}
}
