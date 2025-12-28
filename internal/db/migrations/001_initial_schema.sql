-- 1. Create payments table
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    order_id TEXT NOT NULL,
    customer_id TEXT NOT NULL,
    amount_cents BIGINT NOT NULL,
    currency TEXT NOT NULL DEFAULT 'USD',
    status TEXT NOT NULL,
    
    bank_auth_id TEXT,
    bank_capture_id TEXT,
    bank_void_id TEXT,
    bank_refund_id TEXT,
    
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    authorized_at TIMESTAMP WITH TIME ZONE,
    captured_at TIMESTAMP WITH TIME ZONE,
    voided_at TIMESTAMP WITH TIME ZONE,
    refunded_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    
    attempt_count INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    last_error_category TEXT
);

-- 2. Create idempotency table
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key TEXT PRIMARY KEY,
    payment_id UUID REFERENCES payments(id) ON DELETE CASCADE,
    request_hash TEXT NOT NULL,
    response_payload JSONB,
    locked_at TIMESTAMP WITH TIME ZONE,
    recovery_point TEXT -- Removed trailing comma
);

-- 3. Create indexes for performance optimization
CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);
CREATE INDEX IF NOT EXISTS idx_payments_customer_id ON payments(customer_id);


CREATE INDEX IF NOT EXISTS idx_payments_retry_worker ON payments(next_retry_at) 
WHERE status IN ('CAPTURING', 'VOIDING', 'REFUNDING');