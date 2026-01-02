# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- CoreDNS plugin compliance improvements
- Apache 2.0 LICENSE file
- Ready() method for Kubernetes readiness probes
- Shutdown hook registration
- Tagged switch for qtype handling (performance improvement)
- Comprehensive error handling for JSON encoding and DNS message writing
- GitHub Actions CI/CD workflows
- Security scanning with gosec
- Multi-platform binary builds (linux/darwin, amd64/arm64)
- Automated release process with changelogs
- **Integration tests** for end-to-end validation
- Mock controller HTTP server for integration testing
- Makefile targets: `test-integration`, `test-all`
- **Comprehensive documentation** in `docs/` directory
- Architecture Decision Records (5 ADRs)
- Sequence diagrams for key flows (5 diagrams)
- Performance tuning guide
- Migration guide from other DNS solutions

### Changed
- README.md formatting to CoreDNS man-page style
- Syntax section uses proper CoreDNS conventions
- Examples use reserved domains (example.org)

### Fixed
- Linter warnings (QF1003, G104)
- Unhandled errors in webhook handlers
- Unhandled errors in DNS response writing

### Security
- All error paths now properly handled and logged
- gosec security scan passes with 0 issues
- Static analysis (staticcheck) passes with 0 issues

## [1.0.0] - YYYY-MM-DD (Example - update when releasing)

### Added
- Initial release of Elchi GSLB CoreDNS plugin
- Hash-based change detection for DNS records
- Periodic background sync (default: 5 minutes)
- Optional webhook server for instant updates
- Support for A and AAAA record types
- Thread-safe in-memory cache
- Prometheus metrics export
- Graceful degradation on backend failures
- Comprehensive test suite (53 tests: 48 unit + 5 integration)
- Complete documentation with examples

### Features
- **DNS Serving**: Fast in-memory cache lookup (<1ms latency)
- **Synchronization**: Hash-based change detection with backend
- **Webhooks**: Optional instant update endpoint
- **Health Monitoring**: Health check endpoint with status reporting
- **Record Inspection**: Records endpoint for debugging
- **Metrics**: Prometheus metrics for observability
- **Security**: Secret-based authentication for all API calls
- **Reliability**: Continues serving stale data on backend failures

[Unreleased]: https://github.com/cloudnativeworks/elchi-gslb/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/cloudnativeworks/elchi-gslb/releases/tag/v1.0.0
