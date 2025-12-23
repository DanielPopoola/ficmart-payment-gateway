# Service Layer Test Cases

## Common DSL Functions

### Setup
- `paymentRepositoryMocked`: Initializes a mock payment repository.
- `bankPortMocked`: Initializes a mock bank client.
- `serviceInitialized`: Creates the service instance with mocks.
- `existingPayment`: Pre-seeds the repository with a payment in a specific state.

### Actions
- `authorizeCalled`: Calls `service.Authorize`.
- `captureCalled`: Calls `service.Capture`.
- `refundCalled`: Calls `service.Refund`.
- `voidCalled`: Calls `service.Void`.

### Assertions
- `expectSuccess`: Verifies no error returned and payment returned.
- `expectError`: Verifies an error is returned.
- `expectPaymentStatus`: Verifies the returned payment has the expected status.
- `expectRepoCall`: Verifies the repository methods were called as expected.
- `expectBankCall`: Verifies the bank methods were called.

## 1. Authorization Service

// 1. Successful Authorization
//
// paymentRepositoryMocked
// bankPortMocked
// serviceInitialized
//
// authorizeCalled
//
// expectSuccess
// expectPaymentStatus(StatusAuthorized)
// expectRepoCall("CreatePayment")
// expectBankCall("Authorize")
// expectRepoCall("UpdatePayment")

// 2. Authorization with Invalid Amount
//
// serviceInitialized
//
// authorizeCalled(amount=-100)
//
// expectError("INVALID_AMOUNT")

// 3. Bank Decline
//
// paymentRepositoryMocked
// bankPortMocked(returns error)
// serviceInitialized
//
// authorizeCalled
//
// expectError
// expectPaymentStatus(StatusFailed) // Checked via repo inspection or returned object if logic allows

## 2. Capture Service

// 1. Successful Capture
//
// paymentRepositoryMocked
// bankPortMocked
// serviceInitialized
// existingPayment(StatusAuthorized)
//
// captureCalled
//
// expectSuccess
// expectPaymentStatus(StatusCaptured)
// expectBankCall("Capture")

// 2. Capture Unauthorized Payment
//
// paymentRepositoryMocked
// serviceInitialized
// existingPayment(StatusPending)
//
// captureCalled
//
// expectError("INVALID_STATE")

## 3. Void Service

// 1. Successful Void
//
// paymentRepositoryMocked
// bankPortMocked
// serviceInitialized
// existingPayment(StatusAuthorized)
//
// voidCalled
//
// expectSuccess
// expectPaymentStatus(StatusVoided)
// expectBankCall("Void")

## 4. Refund Service

// 1. Successful Refund
//
// paymentRepositoryMocked
// bankPortMocked
// serviceInitialized
// existingPayment(StatusCaptured)
//
// refundCalled
//
// expectSuccess
// expectPaymentStatus(StatusRefunded)
// expectBankCall("Refund")
