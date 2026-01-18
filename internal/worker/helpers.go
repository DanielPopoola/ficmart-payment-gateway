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
	callBank func(ctx context.Context, idempotencyKey string) (any, error),
	applyResponse func(payment *domain.Payment, response any) error,
) error {
	resp, err := callBank(ctx, idempotencyKey)
	if err != nil {
		if hferr := services.HandleBankFailure(
			ctx,
			w.db,
			w.paymentRepo,
			w.idempotencyRepo,
			payment,
			idempotencyKey,
			err,
		); hferr != nil {
			if application.IsRetryable(hferr) {
				return w.scheduleRetry(ctx, payment)
			}
			return hferr
		}

		return err
	}

	if err := applyResponse(payment, resp); err != nil {
		return err
	}

	return services.FinalizePayment(
		ctx,
		w.db,
		w.paymentRepo,
		w.idempotencyRepo,
		payment,
		idempotencyKey,
		resp,
	)
}

func (w *RetryWorker) scheduleRetry(ctx context.Context, payment *domain.Payment) error {
	backoff := w.calculateBackoff(payment.AttemptCount)
	payment.ScheduleRetry(backoff)
	return w.paymentRepo.Update(ctx, nil, payment)
}

func (w *RetryWorker) calculateBackoff(attemptCount int) time.Duration {
	exponentialMinutes := 1 << attemptCount
	cappedMinutes := min(exponentialMinutes, int(w.maxBackoff))

	return time.Duration(cappedMinutes) * time.Minute
}
