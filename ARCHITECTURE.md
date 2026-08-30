# Architecture & Technical Design

This document details the architectural principles, domain modeling, BPC SmartVista platform integration semantics, and security controls for `go-satim`.

---

## 1. Domain Modeling & Precision

### Minor Currency Units (`AmountMinor int64`)
All monetary figures are represented as `int64` minor units (centimes for DZD). For example, 1,000.00 DZD is represented as `100000`. Float values and implicit conversions are excluded from the core SDK to eliminate floating-point rounding hazards in financial operations.

### Domestic Currency Scope
SATIM functions as Algeria's domestic card payment switch (CIB and Edahabia cards) and processes transactions in Algerian Dinars (DZD). The currency field is fixed to numeric code `012` across all API payloads.

### Cryptographic Order Number Generation
SATIM requires a 10-digit integer for `orderNumber`. When an order number is not explicitly provided, the SDK uses `crypto/rand` to generate a random 10-digit number (`[1000000000, 9999999999]`), preventing collision and enumeration vulnerabilities.

### Stateless Concurrency
The `Client` is stateless and safe for concurrent use across goroutines. Every I/O operation accepts an explicit request struct (`RegisterOrderRequest`, `ConfirmRequest`, `GetStatusRequest`, `RefundRequest`) and takes a `context.Context` as its first parameter.

---

## 2. BPC REST Gateway Semantics

SATIM runs on BPC Group's SmartVista payment engine. The SDK interfaces with its REST endpoints:

1. **`Register` (`/register.do`)**:
   - Initiates an immediate payment session.
   - Generates and returns the payment gateway URL (`FormURL`) and `OrderID`.
   - The web application redirects the cardholder to `FormURL`.
2. **`Confirm` (`/confirmOrder.do`)**:
   - Finalizes authorization upon the cardholder's return from the payment gateway.
   - Transitions the transaction to the confirmed state.
3. **`GetStatus` (`/getOrderStatus.do`)**:
   - Idempotent, read-only endpoint.
   - Inspects the current state of an order without side effects.
   - Used for periodic status polling, payment reconciliation, and background verification.
4. **`Refund` (`/refund.do`)**:
   - Initiates a partial or full refund for an authorized order.

---

## 3. Idempotency & Failure Recovery

- **Read Operations (`GetStatus`)**: Safe to retry on transient transport errors (e.g. network drops or HTTP 5xx responses). The client retries up to 2 times automatically.
- **Mutation Operations (`Register`, `Confirm`, `Refund`)**: Zero automatic retries. If a network interruption occurs during a mutation call, automatic retries could lead to duplicate orders or duplicate refunds.
- **Recovery Procedure**: When `Register`, `Confirm`, or `Refund` encounters a transport error with an unknown state, callers must query `GetStatus(orderID)` using the order identifier to determine the transaction's true state.

---

## 4. Platform Constraints & Error Taxonomy

The SDK enforces the following parameters defined by the SATIM/BPC specification:

- **Order Number Format**: Exactly 10 digits (`1000000000` to `9999999999`).
- **Session Timeout Range**: Between 600 seconds (10 minutes) and 86,400 seconds (24 hours).
- **Description Limit**: Maximum 598 characters.
- **Order States**:
  - `0`: Registered, awaiting payment
  - `1`: Pre-authorization held (two-phase)
  - `2`: Successfully authorized and deposited
  - `3`: Authorization reversed or declined
  - `4`: Transaction refunded
  - `5`: 3D-Secure ACS verification in progress
  - `6`: Authorization declined
- **Error Code Mapping**:
  - `1`: `ErrOrderAlreadyExists` (Duplicate order number)
  - `5`: `ErrInvalidCredentials` (Access denied / invalid credentials or terminal ID)
  - `6`: `ErrOrderNotFound` (Unknown order ID)
  - `7`: `ErrSystemError` (Gateway system failure)

---

## 5. Security & Privacy Controls

- **Mandatory Server-Side Verification**: Query parameters on redirect URLs can be manipulated by clients. Verification via `Confirm` or `GetStatus` on the server is required before fulfilling orders.
- **Credential Protection**: `Credentials` implements `slog.LogValuer`, `fmt.Stringer`, and `fmt.GoStringer`. Passwords are redacted as `[REDACTED]` in all log events and formatting verbs.
- **PAN Masking**: The `Pan` field contains SATIM's standard masked PAN format (e.g. `628058******1234`).
- **TLS Verification**: Standard TLS verification is enabled by default.

---

## 6. Testing Strategy

SATIM does not provide a public sandbox environment without a formal merchant agreement via CIBWeb.dz and GIE Monétique.

All unit and integration tests run against an internal `httptest.Server` test harness that simulates BPC REST API endpoints, response payloads, error codes, and network transport edge cases.
