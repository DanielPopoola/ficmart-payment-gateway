package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/bank"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/handler"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/adapters/postgres"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/domain"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/ports"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/core/service"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/worker"
	"github.com/google/uuid"
)

func setupIntegration(t *testing.T) (*postgres.DB, *handler.PaymentHandler, *config.Config, ports_collection) {
	// Set env vars for config loader using double underscore for nesting
	os.Setenv("GATEWAY_PRIMARY__ENV", "test")
	os.Setenv("GATEWAY_SERVER__PORT", "8081")
	os.Setenv("GATEWAY_SERVER__READ_TIMEOUT", "15s")
	os.Setenv("GATEWAY_SERVER__WRITE_TIMEOUT", "15s")
	os.Setenv("GATEWAY_SERVER__IDLE_TIMEOUT", "60s")
	
	os.Setenv("GATEWAY_DATABASE__HOST", "localhost")
	os.Setenv("GATEWAY_DATABASE__PORT", "5432")
	os.Setenv("GATEWAY_DATABASE__USER", "postgres")
	os.Setenv("GATEWAY_DATABASE__PASSWORD", "postgres")
	os.Setenv("GATEWAY_DATABASE__NAME", "payment_gateway")
	os.Setenv("GATEWAY_DATABASE__SSL_MODE", "disable")
	os.Setenv("GATEWAY_DATABASE__MAX_OPEN_CONNS", "25")
	os.Setenv("GATEWAY_DATABASE__MAX_IDLE_CONNS", "5")
	os.Setenv("GATEWAY_DATABASE__CONN_MAX_LIFETIME", "5m")
	os.Setenv("GATEWAY_DATABASE__CONN_MAX_IDLE_TIME", "5m")
	
	os.Setenv("GATEWAY_BANK_CLIENT__BANK_BASE_URL", "http://localhost:8787")
	os.Setenv("GATEWAY_BANK_CLIENT__BANK_CONN_TIMEOUT", "30s")
	
	os.Setenv("GATEWAY_RETRY__BASE_DELAY", "1")
	os.Setenv("GATEWAY_RETRY__MAX_RETRIES", "3")
	
	os.Setenv("GATEWAY_WORKER__INTERVAL", "1s")
	os.Setenv("GATEWAY_WORKER__BATCH_SIZE", "10")

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	db, err := postgres.Connect(context.Background(), &cfg.Database, slog.Default())
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}

	repo := postgres.NewPaymentRepository(db)
	baseBankClient := bank.NewBankClient(cfg.BankClient)
	bankClient := bank.NewRetryBankClient(baseBankClient, cfg.Retry)

	authService := service.NewAuthorizationService(repo, bankClient)
	capService := service.NewCaptureService(repo, bankClient)
	voidService := service.NewVoidService(repo, bankClient)
	refService := service.NewRefundService(repo, bankClient)
	queryService := service.NewPaymentQueryService(repo)

	h := handler.NewPaymentHandler(authService, capService, refService, voidService, queryService)

	return db, h, cfg, ports_collection{
		repo:        repo,
		bankClient:  bankClient,
		authService: authService,
		capService:  capService,
		voidService: voidService,
		refService:  refService,
	}
}

type ports_collection struct {
	repo        ports.PaymentRepository
	bankClient  ports.BankPort
	authService *service.AuthorizationService
	capService  *service.CaptureService
	voidService *service.VoidService
	refService  *service.RefundService
}

func TestIntegration_FullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, h, _, _ := setupIntegration(t)
	defer db.Close()

	idemKey := uuid.New().String()

	// 1. Authorize
	authReq := handler.AuthorizeRequest{
		OrderID:     "order-" + uuid.New().String(),
		CustomerID:  "cust-" + uuid.New().String(),
		Amount:      1000,
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}
	
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/authorize", toJSON(authReq))
	r.Header.Set("Idempotency-Key", idemKey)
	h.HandleAuthorize(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("Authorize failed: %d %s", w.Code, w.Body.String())
	}

	var authResp handler.APIResponse
	json.Unmarshal(w.Body.Bytes(), &authResp)
	paymentData := authResp.Data.(map[string]interface{})
	paymentID := paymentData["ID"].(string)

	// 2. Capture
	idemKeyCap := uuid.New().String()
	capReq := handler.CaptureRequest{
		PaymentID: paymentID,
		Amount:    1000,
	}
	w = httptest.NewRecorder()
	r = httptest.NewRequest("POST", "/capture", toJSON(capReq))
	r.Header.Set("Idempotency-Key", idemKeyCap)
	h.HandleCapture(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("Capture failed: %d %s", w.Code, w.Body.String())
	}

	// 3. Verify final state
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/payments/order/"+authReq.OrderID, nil)
	r.SetPathValue("orderID", authReq.OrderID)
	h.HandleGetPaymentByOrder(w, r)
	
	if w.Code != http.StatusOK {
		t.Fatalf("GetPayment failed: %d %s", w.Code, w.Body.String())
	}
}

func TestIntegration_ConcurrentDoubleSpend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, h, _, _ := setupIntegration(t)
	defer db.Close()

	// 1. Authorize
	idemKeyAuth := uuid.New().String()
	authReq := handler.AuthorizeRequest{
		OrderID:     "order-" + uuid.New().String(),
		CustomerID:  "cust-" + uuid.New().String(),
		Amount:      5000,
		CardNumber:  "4111111111111111",
		CVV:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}
	wAuth := httptest.NewRecorder()
	rAuth := httptest.NewRequest("POST", "/authorize", toJSON(authReq))
	rAuth.Header.Set("Idempotency-Key", idemKeyAuth)
	h.HandleAuthorize(wAuth, rAuth)

	if wAuth.Code != http.StatusCreated {
		t.Fatalf("Initial Authorize failed: %d %s", wAuth.Code, wAuth.Body.String())
	}

	var authResp handler.APIResponse
	json.Unmarshal(wAuth.Body.Bytes(), &authResp)
	paymentData := authResp.Data.(map[string]interface{})
	paymentID := paymentData["ID"].(string)

	// 2. Concurrent Captures
	const numRequests = 5
	idemKeyCap := uuid.New().String()
	capReq := handler.CaptureRequest{
		PaymentID: paymentID,
		Amount:    5000,
	}

	var wg sync.WaitGroup
	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/capture", toJSON(capReq))
			r.Header.Set("Idempotency-Key", idemKeyCap)
			h.HandleCapture(w, r)
			results <- w.Code
		}()
	}

	wg.Wait()
	close(results)

	// 3. One should succeed with 200, others might return 200 (if polling worked) or 202/409 (if polling timed out)
	// But in our case, with 5 requests and local DB, they should all eventually get the result.
	for code := range results {
		if code != http.StatusOK && code != http.StatusAccepted && code != http.StatusConflict {
			t.Errorf("Unexpected concurrent request status: %d", code)
		}
	}
}

func TestIntegration_CrashSimulationReconciliation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	db, _, cfg, pc := setupIntegration(t)
	defer db.Close()

	// 1. Create a "stuck" PENDING payment manually in DB
	// Simulate crash AFTER bank call: we have a BankAuthID but status is still PENDING.
	paymentID := uuid.New()
	idemKey := "idem-crash-" + uuid.New().String()
	orderID := "order-crash-" + uuid.New().String()
	amount := int64(1000)
	customerID := "cust-crash"
	
	// First, let's actually get a real BankAuthID from the bank to be sure reconciliation works
	bankReq := domain.BankAuthorizationRequest{
		Amount:      amount,
		CardNumber:  "4111111111111111",
		Cvv:         "123",
		ExpiryMonth: 12,
		ExpiryYear:  2030,
	}
	bankResp, err := pc.bankClient.Authorize(context.Background(), bankReq, idemKey)
	if err != nil {
		t.Fatalf("failed to get real bank auth: %v", err)
	}

	p := &domain.Payment{
		ID:             paymentID,
		OrderID:        orderID,
		CustomerID:     customerID,
		AmountCents:    amount,
		Currency:       "USD",
		Status:         domain.StatusPending,
		IdempotencyKey: idemKey,
		BankAuthID:     &bankResp.AuthorizationID, // Seeding the ID we just got
		CreatedAt:      time.Now().Add(-2 * time.Minute),
		UpdatedAt:      time.Now().Add(-2 * time.Minute),
	}
	
	// Create request hash
	hashInput := fmt.Sprintf("%s|%d|%s", orderID, amount, customerID)
	hashBytes := sha256.Sum256([]byte(hashInput))
	requestHash := hex.EncodeToString(hashBytes[:])

	// Create idempotency key record
	err = pc.repo.CreateIdempotencyKey(context.Background(), &domain.IdempotencyKey{
		Key:         idemKey,
		RequestHash: requestHash,
		LockedAt:    time.Now().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to seed idempotency key: %v", err)
	}
	
	err = pc.repo.CreatePayment(context.Background(), p)
	if err != nil {
		t.Fatalf("failed to seed payment: %v", err)
	}

	// 2. Run the worker
	logger := slog.Default()
	reconciler := worker.NewReconciler(
		pc.repo,
		pc.bankClient,
		pc.authService,
		pc.capService,
		pc.voidService,
		pc.refService,
		cfg.Worker.Interval,
		cfg.Worker.BatchSize,
		logger,
	)

	reconciler.RunOnce(context.Background())

	// 3. Verify payment is now AUTHORIZED
	pUpdated, err := pc.repo.FindByID(context.Background(), paymentID)
	if err != nil {
		t.Fatalf("failed to fetch updated payment: %v", err)
	}

	if pUpdated.Status != domain.StatusAuthorized {
		t.Errorf("Expected status AUTHORIZED after reconciliation, got %s", pUpdated.Status)
	}
}

func toJSON(v interface{}) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}
