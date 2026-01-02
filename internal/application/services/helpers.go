package services

import (
	"crypto/sha256"
	"fmt"
)

func ComputeHash(v interface{}) string {
	data := fmt.Sprintf("%+v", v)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

/*
func WaitForCompletion(ctx context.Context, idempotencyKey string) (*domain.Payment, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, application.NewTimeoutError("")
		case <-timeout:
			return nil, application.NewTimeoutError("")
		case <-ticker.C:
			key, err := idempotencyRepo.FindByKey(ctx, idempotencyKey)
			if err != nil {
				return nil, application.NewInternalError(err)
			}

			if key.LockedAt == nil {
				payment, err := s.paymentRepo.FindByID(ctx, key.PaymentID)
				if err != nil {
					return nil, application.NewInternalError(err)
				}
				return payment, nil
			}

			if time.Since(*key.LockedAt) > 5*time.Minute {
				return nil, application.NewRequestProcessingError()
			}
		}
	}
}
*/
