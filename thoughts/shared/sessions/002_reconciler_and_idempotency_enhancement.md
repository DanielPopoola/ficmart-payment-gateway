---
date: 2025-12-23T11:30:00Z
feature: Reconciler and Idempotency Enhancement
plan: thoughts/shared/plans/002_complete_gateway.md
research: thoughts/shared/research/001_authorize_handler_pattern.md
status: in_progress
last_commit: WIP
---

# Session Summary: Reconciler and Idempotency Enhancement

## Session Duration
- Started: 2025-12-23T11:00:00Z
- Ended: 2025-12-23T11:30:00Z
- Duration: 30 minutes

## Objectives
- Enhance idempotency storage to support full cached responses.
- Prepare the domain and bank client for the background worker and lazy expiration.

## Accomplishments
- **Schema Evolution**: Added columns for `response_payload` and `status_code` to support high-fidelity replays.
- **Domain Refinement**: Introduced `StatusRefunding` for better state tracking during the refund lifecycle.
- **Bank Client Expansion**: Added `GetAuthorization` support to allow the reconciler to verify state directly with the bank.
- **Repository Support**: Implemented `UpdateIdempotencyKey` to persist transaction results.

## Discoveries
- Confirmed that the bank should be the source of truth for expiration, meaning the worker must proactively call the bank for authorization statuses.
- Identified that `RefundService` was using `StatusCaptured` as an intermediate state; updated the plan to use `StatusRefunding` for consistency with `CAPTURING` and `VOIDING`.

## Decisions Made
- **Lazy Expiration**: Handlers will defer expiration rejection to the bank, while the worker will mark states based on bank confirmation.
- **Atomic Results**: Services will now be responsible for updating the idempotency record with the final response within their completion logic.

## Open Questions
- None.

## File Changes
```bash
migrations/002_enhance_idempotency.sql      # New columns & indices
internal/core/domain/idempotency.go         # Struct updates
internal/core/domain/payment.go             # State machine & StatusRefunding
internal/core/ports/bank.go                 # GetAuthorization interface
internal/core/ports/repository.go           # UpdateIdempotencyKey interface
internal/adapters/bank/client.go            # GetAuthorization impl
internal/adapters/bank/retry.go             # GetAuthorization retry wrapper
internal/adapters/postgres/repository.go    # DB implementation for new fields
```

## Ready to Resume
To continue this work:
1. Update `Authorize`, `Capture`, `Void`, and `Refund` services to use `UpdateIdempotencyKey`.
2. Implement the `internal/worker` package.
3. Perform the final wiring in `cmd/gateway`.
