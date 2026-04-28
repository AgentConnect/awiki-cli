SHELL := /usr/bin/env bash

.PHONY: help test coverage coverage-check

help:
	@echo "Available targets:"
	@echo "  make test           Run the Go unit test suite."
	@echo "  make coverage       Run tests with coverage output and keep the profile."
	@echo "  make coverage-check Run tests with coverage output and enforce thresholds."

test:
	@./scripts/test-unit.sh

coverage:
	@AWIKI_CLI_COVERPROFILE="$${AWIKI_CLI_COVERPROFILE:-coverage/unit-cover.out}" ./scripts/test-unit-cover.sh --show-profile

coverage-check:
	@AWIKI_CLI_COVERPROFILE="$${AWIKI_CLI_COVERPROFILE:-coverage/unit-cover.out}" ./scripts/test-unit-cover.sh --check --show-profile
