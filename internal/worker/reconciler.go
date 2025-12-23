package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
)

type ReconcilerService interface {
	Reconcile(ctx context.Context, p *domain.Payment) error
}

type Reconciler struct {
	repo        ports.PaymentRepository
	bankClient  ports.BankPort
	authService ReconcilerService
	capService  ReconcilerService
	voidService ReconcilerService
	refService  ReconcilerService
	interval    time.Duration
	batchSize   int
	logger      *slog.Logger
}

func NewReconciler(
	repo ports.PaymentRepository,
	bankClient ports.BankPort,
	authService ReconcilerService,
	capService ReconcilerService,
	voidService ReconcilerService,
	refService ReconcilerService,
	interval time.Duration,
	batchSize int,
	logger *slog.Logger,
) *Reconciler {
	return &Reconciler{
		repo:        repo,
		bankClient:  bankClient,
		authService: authService,
		capService:  capService,
		voidService: voidService,
		refService:  refService,
		interval:    interval,
		batchSize:   batchSize,
		logger:      logger,
	}
}

func (r *Reconciler) Start(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("starting background reconciler", "interval", r.interval, "batch_size", r.batchSize)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("stopping background reconciler")
			return
		case <-ticker.C:
			r.run(ctx)
		}
	}
}

// RunOnce executes a single reconciliation cycle.
func (r *Reconciler) RunOnce(ctx context.Context) {
	r.run(ctx)
}

func (r *Reconciler) run(ctx context.Context) {
	r.reconcileStuckPayments(ctx)
}

func (r *Reconciler) reconcileStuckPayments(ctx context.Context) {
	pending, err := r.repo.FindPendingPayments(ctx, 1*time.Minute, r.batchSize)
	if err != nil {
		r.logger.Error("failed to fetch pending payments", "error", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	r.logger.Info("reconciling stuck payments", "count", len(pending))

	for _, pCheck := range pending {
		// Fetch full payment record
		p, err := r.repo.FindByID(ctx, pCheck.ID)
		if err != nil {
			r.logger.Error("failed to fetch payment for reconciliation", "id", pCheck.ID, "error", err)
			continue
		}

		var svc ReconcilerService
		switch p.Status {
		case domain.StatusPending:
			svc = r.authService
		case domain.StatusCapturing:
			svc = r.capService
		case domain.StatusVoiding:
			svc = r.voidService
		case domain.StatusRefunding:
			svc = r.refService
		case domain.StatusAuthorized:
			// Authorized payments are checked for expiration
			r.checkExpiration(ctx, p)
			continue
		default:
			continue
		}

		if svc != nil {
			if err := svc.Reconcile(ctx, p); err != nil {
				r.logger.Error("reconciliation failed for payment", "id", p.ID, "status", p.Status, "error", err)
			} else {
				r.logger.Info("successfully reconciled payment", "id", p.ID, "new_status", p.Status)
			}
		}
	}
}

func (r *Reconciler) checkExpiration(ctx context.Context, p *domain.Payment) {
	// If it's authorized, check with the bank if it has expired
	if p.Status != domain.StatusAuthorized || p.BankAuthID == nil {
		return
	}

	// We only check if our local expires_at has passed
	if p.ExpiresAt != nil && p.ExpiresAt.After(time.Now()) {
		return
	}

	r.logger.Info("checking expiration with bank", "id", p.ID, "auth_id", *p.BankAuthID)

	bankResp, err := r.bankClient.GetAuthorization(ctx, *p.BankAuthID)
	if err != nil {
		// If bank says not found, it's definitely expired or invalid
		if isNotFound(err) {
			r.markExpired(ctx, p)
		}
		return
	}

	if bankResp.ExpiresAt.Before(time.Now()) {
		r.markExpired(ctx, p)
	}
}

func (r *Reconciler) markExpired(ctx context.Context, p *domain.Payment) {
	err := r.repo.WithTx(ctx, func(txRepo ports.PaymentRepository) error {
		payment, err := txRepo.FindByIDForUpdate(ctx, p.ID)
		if err != nil {
			return err
		}
		if payment.Status != domain.StatusAuthorized {
			return nil
		}
		payment.Status = domain.StatusExpired
		return txRepo.UpdatePayment(ctx, payment)
	})

	if err != nil {
		r.logger.Error("failed to mark payment as expired", "id", p.ID, "error", err)
	} else {
		r.logger.Info("marked payment as expired", "id", p.ID)
	}
}

func isNotFound(err error) bool {
	var bankErr *bank.BankError
	if errors.As(err, &bankErr) {
		return bankErr.StatusCode == 404
	}
	return false
}
