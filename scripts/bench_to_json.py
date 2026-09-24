#!/usr/bin/env python3
"""Parse `go test -bench . -benchmem` output into a baseline metrics JSON.

Usage: bench_to_json.py <raw_go_test_output.txt> <output.json>

Each `go test -bench` line looks like:
  BenchmarkBuildTree/leaves=8-8   123456   963 ns/op   128 B/op   4 allocs/op

We capture name, iterations, ns/op, B/op, allocs/op per benchmark case.
"""
import json
import re
import sys
from datetime import datetime, timezone

LINE_RE = re.compile(
    r"^(?P<name>Benchmark\S+)\s+"
    r"(?P<iters>\d+)\s+"
    r"(?P<ns_op>[\d.]+)\s+ns/op"
    r"(?:\s+(?P<b_op>[\d.]+)\s+B/op)?"
    r"(?:\s+(?P<allocs_op>[\d.]+)\s+allocs/op)?"
)


def parse(raw_text: str):
    results = []
    for line in raw_text.splitlines():
        m = LINE_RE.match(line.strip())
        if not m:
            continue
        d = m.groupdict()
        results.append(
            {
                "name": d["name"],
                "iterations": int(d["iters"]),
                "ns_per_op": float(d["ns_op"]),
                "bytes_per_op": float(d["b_op"]) if d["b_op"] else None,
                "allocs_per_op": float(d["allocs_op"]) if d["allocs_op"] else None,
            }
        )
    return results


def main():
    if len(sys.argv) != 3:
        print(__doc__)
        sys.exit(1)
    raw_path, out_path = sys.argv[1], sys.argv[2]
    with open(raw_path, "r") as f:
        raw_text = f.read()

    benchmarks = parse(raw_text)
    if not benchmarks:
        print("warning: no benchmark lines matched — writing empty baseline")

    payload = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "go_version": "1.24",
        "benchmarks": benchmarks,
    }
    with open(out_path, "w") as f:
        json.dump(payload, f, indent=2)
        f.write("\n")
    print(f"wrote {len(benchmarks)} benchmark results to {out_path}")


if __name__ == "__main__":
    main()
