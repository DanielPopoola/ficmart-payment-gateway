package worker

import (
	"context"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/application/services"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
)

func (w *RetryWorker) resumeOperation(
	ctx context.Context,
	payment *domain.Payment,
	idempotencyKey string,
	callBank func(ctx context.Context, idempotencyKey string) (interface{}, error),
	applyResponse func(payment *domain.Payment, response interface{}) error,
) error {
	resp, err := callBank(ctx, idempotencyKey)
	if err != nil {
		if err := services.HandleBankFailure(
			ctx,
			w.db,
			w.paymentRepo,
			w.idempotencyRepo,
			payment,
			idempotencyKey,
			err,
		); err != nil {
			return err
		}

		if application.IsRetryable(err) {
			return w.scheduleRetry(ctx, payment, err)
		}
		return err
	}

	if err := applyResponse(payment, resp); err != nil {
		return err
	}

	return services.FinalizePaymentSuccess(
		ctx,
		w.db,
		w.paymentRepo,
		w.idempotencyRepo,
		payment,
		idempotencyKey,
		resp,
	)
}

func (w *RetryWorker) scheduleRetry(ctx context.Context, payment *domain.Payment, lastErr error) error {
	category := application.CategorizeError(lastErr)

	payment.ScheduleRetry(
		time.Duration(1<<payment.AttemptCount)*time.Minute,
		string(category),
	)
	return w.paymentRepo.Update(ctx, nil, payment)
}
