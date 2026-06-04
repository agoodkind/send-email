# Lint is centralized in go-makefile. Do NOT define project-local lint,
# deadcode, audit, fmt, vet, or staticcheck targets here. They duplicate
# the central pipeline and let agents bypass strict rules. Run `make help`
# for the canonical entry points (build/check/lint/fmt) and per-linter
# sub-targets (lint-golangci, lint-format, lint-gocyclo, lint-deadcode,
# staticcheck-extra). Refresh baselines via the matching *-baseline target.
#
# send-email Makefile.
# Build/lint/release pipeline lives in go-makefile and is fetched at runtime.

# Identity. No VPKG: send-email is a tiny binary without a version package.
BINARY := send-email
CMD    := .

# send-email installs system-wide. go-mk install copies into INSTALL_DIR
# through sudo when the directory is not writable.
INSTALL_DIR := /opt/scripts

# Pipeline modules
GO_MK_MODULES := go-build.mk go-release.mk

include bootstrap.mk

.DEFAULT_GOAL := check
