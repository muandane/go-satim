# Migration Guide: PHP to Go (1.27)

This document records the architectural decisions, behavior changes, and API mappings made when porting `PiteurStudio/satim-php` to the Go module `github.com/muandane/go-satim`.

---

## 1. Overview of Architectural Decisions

### Money Precision: `int64` Minor Units
- **PHP Behavior:** Accepted amounts as integers or floats and multiplied by 100 (`amount * 100`).
- **Go Decision:** The SDK accepts `AmountMinor int64` exclusively in minor units (centimes). For example, 1000.00 DZD is represented as `100000`. Float values and implicit conversions are completely removed to prevent floating-point rounding errors in financial transactions.

### Currency Scope: DZD Only
- **PHP Behavior:** Included configuration arrays for `DZD`, `USD`, and `EUR`.
- **Go Decision:** SATIM operates as Algeria's domestic card payment switch and processes transactions in Algerian Dinars (DZD). The currency field is hardcoded to numeric code `012` in all API payloads and removed from the public configuration surface.

### Order Number Generation: `crypto/rand`
- **PHP Behavior:** Used `mt_rand(1000000000, 9999999999)`.
- **Go Decision:** Auto-generated order numbers use `crypto/rand` to generate cryptographically secure 10-digit integers. This prevents order enumeration and predictable collisions.

### HTTP Side Effects: Removal of `redirect()`
- **PHP Behavior:** Provided a `redirect()` method that set HTTP headers (`header("Location: ...")`) and called `exit`.
- **Go Decision:** `redirect()` is dropped. `RegisterOrderResponse` returns the payment gateway URL in the `FormURL` field. The calling web application is responsible for performing the HTTP redirect in its own router or handler.

### Concurrency and State: Stateless Client
- **PHP Behavior:** Mutated internal state across fluent method calls (`$satim->amount()->register()`).
- **Go Decision:** The Go `Client` is stateless and safe for concurrent use by multiple goroutines. Each payment operation accepts an explicit request struct (`RegisterOrderRequest`, `ConfirmRequest`, `GetStatusRequest`, `RefundRequest`) and takes a `context.Context` as its first parameter.

---

## 2. Semantics: `Confirm` vs `GetStatus`

The original PHP library provided both `confirm($orderId)` and `status($orderId)`. These map to distinct BPC SmartVista REST endpoints with different operational semantics:

1. **`Confirm` (`/confirmOrder.do`)**:
   - In BPC's payment gateway, `/confirmOrder.do` finalizes the transaction after the cardholder returns from the hosted payment page.
   - It performs authorization finalization and transitions the order state to confirmed.
   - This endpoint is a state-altering mutation call.
2. **`GetStatus` (`/getOrderStatus.do`)**:
   - `/getOrderStatus.do` is an idempotent, read-only endpoint.
   - It inspects the current transaction state without modifying it.
   - It is intended for periodic status polling, payment reconciliation, and verification upon customer return.

---

## 3. Idempotency, Retries, and Failure Recovery

- **Read Operations (`GetStatus`)**: Safe to retry on transient transport errors (e.g. dropped connections or HTTP 5xx responses). The Go client retries up to 2 times.
- **Mutation Operations (`Register`, `Confirm`, `Refund`)**: Zero automatic retries. If a network error occurs during a mutation call, automatic retries could result in duplicate orders or duplicate refunds.
- **Recovery Strategy**: When `Register`, `Confirm`, or `Refund` returns a network error with an unknown state, callers must query `GetStatus(orderID)` to check the actual state of the transaction before taking further action.

---

## 4. Origin of SATIM Constraints

The following constraints are documented in SATIM / BPC technical specifications and preserved in the Go module:

- **10-digit Order Number**: SATIM requires `orderNumber` to be a 10-digit integer (`1000000000` to `9999999999`).
- **Session Timeout (600s to 86400s)**: BPC session timeouts must be between 10 minutes (600 seconds) and 24 hours (86400 seconds).
- **Description Length**: BPC limits the `description` field to 598 characters.
- **Order States**:
  - `0`: Registered, awaiting payment
  - `1`: Pre-authorization held (two-phase)
  - `2`: Successfully authorized and deposited
  - `3`: Authorization reversed or declined
  - `4`: Transaction refunded
  - `5`: 3D-Secure ACS cardholder verification in progress
  - `6`: Authorization declined
- **Error Codes**:
  - `1`: Order number already exists
  - `5`: Access denied (invalid credentials or terminal ID)
  - `6`: Unknown order ID
  - `7`: System error

---

## 5. Security Controls

- **Zero-Trust Redirects**: The package documentation explicitly instructs callers never to trust query parameters received on `returnUrl` or `failUrl`. Server-side verification via `Confirm` or `GetStatus` is mandatory.
- **Credential Protection**: `Credentials` implements `slog.LogValuer`, `fmt.Stringer`, and `fmt.GoStringer`. Passwords are redacted as `[REDACTED]` in all logging and format verbs.
- **PAN Masking**: The `Pan` field returned in transaction responses contains SATIM's standard masked PAN format (e.g. `628058******1234`).
- **TLS Verification**: Standard TLS verification is enabled by default.

---

## 6. Test Suite and Sandbox Constraints

SATIM does not offer a public sandbox environment without a signed commercial agreement via CIBWeb.dz and GIE Monétique. 

All unit and integration tests run against a local `httptest.Server` fake that simulates the BPC REST API endpoints, response payloads, error codes, and transport failures.
