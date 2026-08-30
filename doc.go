// Package satim provides an idiomatic, secure Go SDK for integrating with Algeria's SATIM
// (Société d'Automatisation des Télécompensations et de Monétique) and GIE Monétique payment gateway,
// built on the BPC SmartVista banking engine.
//
// # Domestic Currency & Monetary Precision
//
// SATIM processes transactions exclusively in Algerian Dinars (DZD, numeric currency code 012).
// All monetary amounts are modeled strictly as int64 minor currency units (centimes, where 1.00 DZD = 100 centimes)
// to eliminate floating-point rounding errors in financial transactions.
//
// # Security & Payment Verification
//
// CRITICAL SECURITY REQUIREMENT:
// When a customer is redirected back to ReturnURL or FailURL after completing a payment, the query
// parameters on the redirect URL must NEVER be trusted as proof of payment. Applications MUST verify
// the transaction status server-side using (*Client).Confirm or (*Client).GetStatus before fulfilling
// orders or granting access.
//
// All client credentials implement slog.LogValuer, fmt.Stringer, and fmt.GoStringer to prevent password
// leakage in log files, debug traces, and error outputs.
package satim
