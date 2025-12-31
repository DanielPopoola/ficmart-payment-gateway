package domain_test

import (
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPayment(t *testing.T) {
	t.Run("creates payment successfully", func(t *testing.T) {
		money, err := domain.NewMoney(5000, "USD")
		require.NoError(t, err)

		payment, err := domain.NewPayment("pay-123", "order-456", "cust-789", money)

		require.NoError(t, err)
		assert.Equal(t, "pay-123", payment.ID)
		assert.Equal(t, "order-456", payment.OrderID)
		assert.Equal(t, "cust-789", payment.CustomerID)
		assert.Equal(t, int64(5000), payment.AmountCents)
		assert.Equal(t, "USD", payment.Currency)
		assert.Equal(t, domain.StatusPending, payment.Status)
		assert.NotZero(t, payment.CreatedAt)
	})

	t.Run("rejects empty payment ID", func(t *testing.T) {
		money, _ := domain.NewMoney(5000, "USD")

		_, err := domain.NewPayment("", "order-456", "cust-789", money)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "payment ID is required")
	})

	t.Run("rejects empty order ID", func(t *testing.T) {
		money, _ := domain.NewMoney(5000, "USD")

		_, err := domain.NewPayment("pay-123", "", "cust-789", money)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "order ID is required")
	})

	t.Run("rejects empty customer ID", func(t *testing.T) {
		money, _ := domain.NewMoney(5000, "USD")

		_, err := domain.NewPayment("pay-123", "order-456", "", money)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "customer ID is required")
	})
}

func TestNewMoney(t *testing.T) {
	t.Run("creates money successfully", func(t *testing.T) {
		money, err := domain.NewMoney(5000, "USD")

		require.NoError(t, err)
		assert.Equal(t, int64(5000), money.Amount)
		assert.Equal(t, "USD", money.Currency)
	})

	t.Run("rejects negative amount", func(t *testing.T) {
		_, err := domain.NewMoney(-100, "USD")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "amount cannot be negative")
	})

	t.Run("rejects empty currency", func(t *testing.T) {
		_, err := domain.NewMoney(5000, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "currency is required")
	})
}

func TestPayment_StateTransitions(t *testing.T) {
	t.Run("PENDING -> AUTHORIZED transition", func(t *testing.T) {
		payment := createTestPayment(t)

		err := payment.Authorize("auth-123", time.Now(), time.Now().Add(7*24*time.Hour))

		require.NoError(t, err)
		assert.Equal(t, domain.StatusAuthorized, payment.Status)
		assert.Equal(t, "auth-123", *payment.BankAuthID)
		assert.NotNil(t, payment.AuthorizedAt)
		assert.NotNil(t, payment.ExpiresAt)
	})

	t.Run("PENDING -> FAILED transition", func(t *testing.T) {
		payment := createTestPayment(t)

		err := payment.Fail()

		require.NoError(t, err)
		assert.Equal(t, domain.StatusFailed, payment.Status)
	})

	t.Run("AUTHORIZED -> CAPTURING transition", func(t *testing.T) {
		payment := createAuthorizedPayment(t)

		err := payment.MarkCapturing()

		require.NoError(t, err)
		assert.Equal(t, domain.StatusCapturing, payment.Status)
	})

	t.Run("CAPTURING -> CAPTURED transition", func(t *testing.T) {
		payment := createCapturingPayment(t)

		err := payment.Capture("cap-123", time.Now())

		require.NoError(t, err)
		assert.Equal(t, domain.StatusCaptured, payment.Status)
		assert.Equal(t, "cap-123", *payment.BankCaptureID)
		assert.NotNil(t, payment.CapturedAt)
	})

	t.Run("AUTHORIZED -> VOIDING transition", func(t *testing.T) {
		payment := createAuthorizedPayment(t)

		err := payment.MarkVoiding()

		require.NoError(t, err)
		assert.Equal(t, domain.StatusVoiding, payment.Status)
	})

	t.Run("VOIDING -> VOIDED transition", func(t *testing.T) {
		payment := createVoidingPayment(t)

		err := payment.Void("void-123", time.Now())

		require.NoError(t, err)
		assert.Equal(t, domain.StatusVoided, payment.Status)
		assert.Equal(t, "void-123", *payment.BankVoidID)
		assert.NotNil(t, payment.VoidedAt)
	})

	t.Run("CAPTURED -> REFUNDING transition", func(t *testing.T) {
		payment := createCapturedPayment(t)

		err := payment.MarkRefunding()

		require.NoError(t, err)
		assert.Equal(t, domain.StatusRefunding, payment.Status)
	})

	t.Run("REFUNDING -> REFUNDED transition", func(t *testing.T) {
		payment := createRefundingPayment(t)

		err := payment.Refund("ref-123", time.Now())

		require.NoError(t, err)
		assert.Equal(t, domain.StatusRefunded, payment.Status)
		assert.Equal(t, "ref-123", *payment.BankRefundID)
		assert.NotNil(t, payment.RefundedAt)
	})
}

func TestPayment_InvalidStateTransitions(t *testing.T) {
	t.Run("cannot capture from PENDING", func(t *testing.T) {
		payment := createTestPayment(t)

		err := payment.MarkCapturing()

		assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	})

	t.Run("cannot void from CAPTURED", func(t *testing.T) {
		payment := createCapturedPayment(t)

		err := payment.MarkVoiding()

		assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	})

	t.Run("cannot capture from VOIDED", func(t *testing.T) {
		payment := createVoidedPayment(t)

		err := payment.MarkCapturing()

		assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	})

	t.Run("cannot refund from AUTHORIZED", func(t *testing.T) {
		payment := createAuthorizedPayment(t)

		err := payment.MarkRefunding()

		assert.ErrorIs(t, err, domain.ErrInvalidTransition)
	})
}

func TestPayment_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		status   domain.PaymentStatus
		terminal bool
	}{
		{"PENDING is not terminal", domain.StatusPending, false},
		{"AUTHORIZED is not terminal", domain.StatusAuthorized, false},
		{"CAPTURING is not terminal", domain.StatusCapturing, false},
		{"CAPTURED is not terminal", domain.StatusCaptured, false},
		{"VOIDED is terminal", domain.StatusVoided, true},
		{"REFUNDED is terminal", domain.StatusRefunded, true},
		{"EXPIRED is terminal", domain.StatusExpired, true},
		{"FAILED is terminal", domain.StatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payment := createPaymentWithStatus(t, tt.status)

			assert.Equal(t, tt.terminal, payment.IsTerminal())
		})
	}
}

func TestPayment_ScheduleRetry(t *testing.T) {
	t.Run("schedules retry correctly", func(t *testing.T) {
		payment := createTestPayment(t)
		backoff := 2 * time.Minute

		payment.ScheduleRetry(backoff, "TRANSIENT")

		assert.Equal(t, 1, payment.AttemptCount)
		assert.NotNil(t, payment.NextRetryAt)
		assert.Equal(t, "TRANSIENT", *payment.LastErrorCategory)

		expectedRetryTime := time.Now().Add(backoff)
		assert.WithinDuration(t, expectedRetryTime, *payment.NextRetryAt, time.Second)
	})

	t.Run("increments attempt count on multiple retries", func(t *testing.T) {
		payment := createTestPayment(t)

		payment.ScheduleRetry(1*time.Minute, "TRANSIENT")
		payment.ScheduleRetry(2*time.Minute, "TRANSIENT")
		payment.ScheduleRetry(4*time.Minute, "TRANSIENT")

		assert.Equal(t, 3, payment.AttemptCount)
	})
}

func createTestPayment(t *testing.T) *domain.Payment {
	t.Helper()
	money, err := domain.NewMoney(5000, "USD")
	require.NoError(t, err)

	payment, err := domain.NewPayment("pay-123", "order-456", "cust-789", money)
	require.NoError(t, err)

	return payment
}

func createAuthorizedPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createTestPayment(t)
	err := payment.Authorize("auth-123", time.Now(), time.Now().Add(7*24*time.Hour))
	require.NoError(t, err)
	return payment
}

func createCapturingPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createAuthorizedPayment(t)
	err := payment.MarkCapturing()
	require.NoError(t, err)
	return payment
}

func createCapturedPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createCapturingPayment(t)
	err := payment.Capture("cap-123", time.Now())
	require.NoError(t, err)
	return payment
}

func createVoidingPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createAuthorizedPayment(t)
	payment.Status = domain.StatusVoiding
	return payment
}

func createVoidedPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createVoidingPayment(t)
	err := payment.Void("void-123", time.Now())
	require.NoError(t, err)
	return payment
}

func createRefundingPayment(t *testing.T) *domain.Payment {
	t.Helper()
	payment := createCapturedPayment(t)
	err := payment.MarkRefunding()
	require.NoError(t, err)
	return payment
}

func createPaymentWithStatus(t *testing.T, status domain.PaymentStatus) *domain.Payment {
	t.Helper()

	authID := "auth-123"
	captureID := "cap-123"
	voidID := "void-123"
	refundID := "ref-123"
	now := time.Now()
	expiresAt := now.Add(7 * 24 * time.Hour)

	return domain.Reconstitute(
		"pay-123",
		"order-456",
		"cust-789",
		5000,
		"USD",
		status,
		&authID,
		&captureID,
		&voidID,
		&refundID,
		now,
		&now,
		&now,
		&now,
		&now,
		&expiresAt,
		0,
		nil,
		nil,
	)
}
