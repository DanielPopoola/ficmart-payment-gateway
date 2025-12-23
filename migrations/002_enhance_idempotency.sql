-- Add columns to support high-fidelity idempotent replays
ALTER TABLE idempotency_keys 
ADD COLUMN response_payload JSONB,
ADD COLUMN status_code INT,
ADD COLUMN completed_at TIMESTAMP WITH TIME ZONE;

-- Ensure index covers all intermediate states for the Reconciler
DROP INDEX IF EXISTS idx_payments_status_retry;
CREATE INDEX idx_payments_status_retry ON payments(status, next_retry_at) 
WHERE status IN ('PENDING', 'AUTHORIZED', 'CAPTURING', 'VOIDING', 'REFUNDING');