# Changelog

All notable changes to `go-satim` will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
