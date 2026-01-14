package handlers

import (
	"context"
	"net/http"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/api"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/interfaces/rest"
)

func (h *Handlers) GetPaymentByID(
	ctx context.Context,
	request api.GetPaymentByIDRequestObject,
) (api.GetPaymentByIDResponseObject, error) {

	paymentID := request.PaymentID.String()

	payment, err := h.queryService.FindByID(ctx, paymentID)
	if err != nil {
		return mapIdServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapIdServiceErrorToAPIResponse(ctx, err)
	}

	return api.GetPaymentByID200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil

}

func (h *Handlers) GetPaymentsByCustomer(
	ctx context.Context,
	request api.GetPaymentsByCustomerRequestObject,
) (api.GetPaymentsByCustomerResponseObject, error) {

	customerID := request.CustomerID
	limit := request.Params.Limit
	offset := request.Params.Offset

	customerPayment, err := h.queryService.FindByCustomerID(ctx, customerID, limit, offset)
	if err != nil {
		return mapCustomerServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayments(customerPayment)
	if err != nil {
		return mapCustomerServiceErrorToAPIResponse(ctx, err)
	}

	return api.GetPaymentsByCustomer200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func (h *Handlers) GetPaymentByOrder(
	ctx context.Context,
	request api.GetPaymentByOrderRequestObject,
) (api.GetPaymentByOrderResponseObject, error) {

	orderID := request.OrderID

	payment, err := h.queryService.FindByOrderID(ctx, orderID)
	if err != nil {
		return mapOrderServiceErrorToAPIResponse(ctx, err)
	}

	apiPayment, err := rest.ToAPIPayment(payment)
	if err != nil {
		return mapOrderServiceErrorToAPIResponse(ctx, err)
	}

	return api.GetPaymentByOrder200JSONResponse{
		Success: true,
		Data:    apiPayment,
	}, nil
}

func mapIdServiceErrorToAPIResponse(ctx context.Context, err error) (api.GetPaymentByIDResponseObject, error) {
	statusCode, errorResponse := rest.BuildErrorResponse(err)

	switch statusCode {
	case http.StatusNotFound:
		return api.GetPaymentByID404JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.GetPaymentByID500JSONResponse(errorResponse), nil
	default:
		return api.GetPaymentByID500JSONResponse(errorResponse), nil
	}
}

func mapOrderServiceErrorToAPIResponse(ctx context.Context, err error) (api.GetPaymentByOrderResponseObject, error) {
	statusCode, errorResponse := rest.BuildErrorResponse(err)

	switch statusCode {
	case http.StatusNotFound:
		return api.GetPaymentByOrder404JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.GetPaymentByOrder500JSONResponse(errorResponse), nil
	default:
		return api.GetPaymentByOrder500JSONResponse(errorResponse), nil
	}
}

func mapCustomerServiceErrorToAPIResponse(ctx context.Context, err error) (api.GetPaymentsByCustomerResponseObject, error) {
	statusCode, errorResponse := rest.BuildErrorResponse(err)

	switch statusCode {
	case http.StatusNotFound:
		return api.GetPaymentsByCustomer404JSONResponse(errorResponse), nil
	case http.StatusInternalServerError:
		return api.GetPaymentsByCustomer500JSONResponse(errorResponse), nil
	default:
		return api.GetPaymentsByCustomer500JSONResponse(errorResponse), nil
	}
}
