# Changelog

All notable changes to `go-satim` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.1.1 - 2026-09-03

### What's Changed

* test: enhance client tests and add error handling scenarios by @muandane in https://github.com/muandane/go-satim/pull/3
* test: add fuzz tests for order request validation and JSON unmarshalling by @muandane in https://github.com/muandane/go-satim/pull/4
* feat: Go 1.27 modernization, fix OrderNumber loss, and improve error ergonomics by @muandane in https://github.com/muandane/go-satim/pull/6

**Full Changelog**: https://github.com/muandane/go-satim/compare/v0.1.0...v0.1.1

## [Unreleased]

### Added

- Initial standalone release of `go-satim` for Go 1.27.
- Payment registration via `/register.do` with `AmountMinor int64` precision.
- Cryptographically secure 10-digit order number generation via `crypto/rand`.
- Payment confirmation via `/confirmOrder.do` and idempotent status queries via `/getOrderStatus.do`.
- Refund processing via `/refund.do`.
- Structured error mapping matching BPC gateway codes with `errors.Is`.
- Secret and PAN redaction via `slog.LogValuer` and `fmt.Stringer`.
- Strict linter configuration with `golangci-lint` and GitHub Actions CI.
