#!/usr/bin/env python3
"""Validate Go coverage thresholds from a coverprofile."""

from __future__ import annotations

import argparse
import pathlib
import sys

MODULE_PREFIX = "github.com/agentconnect/awiki-cli/"
TOTAL_LABEL = "total"
THRESHOLDS = {
    TOTAL_LABEL: 40.0,
    "internal/cli": 20.0,
    "internal/message": 28.0,
    "internal/identity": 50.0,
    "internal/store": 60.0,
    "internal/doctor": 70.0,
    "internal/docs": 100.0,
    "internal/buildinfo": 100.0,
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check package and total coverage thresholds from a Go coverprofile."
    )
    parser.add_argument("coverprofile", help="Path to the Go coverprofile file.")
    return parser.parse_args()


def normalize_path(raw_path: str) -> str:
    if raw_path.startswith(MODULE_PREFIX):
        return raw_path[len(MODULE_PREFIX) :]
    return raw_path


def load_coverage(path: pathlib.Path) -> tuple[dict[str, tuple[int, int]], tuple[int, int]]:
    package_totals: dict[str, list[int]] = {}
    covered_total = 0
    statement_total = 0

    with path.open("r", encoding="utf-8") as handle:
        for index, line in enumerate(handle):
            line = line.strip()
            if not line:
                continue
            if index == 0:
                if line != "mode: set":
                    raise ValueError(f"unsupported coverprofile mode: {line}")
                continue

            file_range, num_statements_text, count_text = line.rsplit(" ", 2)
            file_name = normalize_path(file_range.split(":", 1)[0])
            package_name = file_name.rsplit("/", 1)[0]
            num_statements = int(num_statements_text)
            covered = num_statements if int(count_text) > 0 else 0

            stats = package_totals.setdefault(package_name, [0, 0])
            stats[0] += covered
            stats[1] += num_statements
            covered_total += covered
            statement_total += num_statements

    frozen_packages = {name: (stats[0], stats[1]) for name, stats in package_totals.items()}
    return frozen_packages, (covered_total, statement_total)


def to_percent(covered: int, total: int) -> float:
    if total <= 0:
        return 0.0
    return (covered / total) * 100.0


def main() -> int:
    args = parse_args()
    coverprofile = pathlib.Path(args.coverprofile)
    if not coverprofile.is_file():
        print(f"coverage file not found: {coverprofile}", file=sys.stderr)
        return 1

    package_totals, total_stats = load_coverage(coverprofile)
    failures: list[tuple[str, float, float]] = []

    total_percent = to_percent(*total_stats)
    if total_percent < THRESHOLDS[TOTAL_LABEL]:
        failures.append((TOTAL_LABEL, total_percent, THRESHOLDS[TOTAL_LABEL]))

    for package_name, threshold in THRESHOLDS.items():
        if package_name == TOTAL_LABEL:
            continue
        percent = to_percent(*package_totals.get(package_name, (0, 0)))
        if percent < threshold:
            failures.append((package_name, percent, threshold))

    print("Coverage summary:")
    print(f"  {TOTAL_LABEL:20} {total_percent:6.1f}% (threshold {THRESHOLDS[TOTAL_LABEL]:.1f}%)")
    for package_name in sorted(name for name in THRESHOLDS if name != TOTAL_LABEL):
        percent = to_percent(*package_totals.get(package_name, (0, 0)))
        print(f"  {package_name:20} {percent:6.1f}% (threshold {THRESHOLDS[package_name]:.1f}%)")

    if failures:
        print("Coverage check failed:", file=sys.stderr)
        for package_name, percent, threshold in failures:
            print(
                f"  {package_name}: {percent:.1f}% < required {threshold:.1f}%",
                file=sys.stderr,
            )
        return 1

    print("Coverage check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
