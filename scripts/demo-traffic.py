#!/usr/bin/env python3
"""Generate a short, read-only API workload for the local Grafana dashboard."""

import argparse
import json
import random
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter


def request(url: str) -> tuple[int, bytes]:
    try:
        with urllib.request.urlopen(url, timeout=10) as response:
            return response.status, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.read()


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--requests", type=int, default=120)
    parser.add_argument("--interval", type=float, default=0.15)
    args = parser.parse_args()
    if args.requests < 1 or args.interval < 0:
        parser.error("--requests must be positive and --interval cannot be negative")

    base = args.base_url.rstrip("/") + "/api/v1"
    status, body = request(base + "/articles?pageSize=50")
    if status != 200:
        raise SystemExit(f"article listing failed: HTTP {status}")
    articles = json.loads(body)["items"]
    if not articles:
        raise SystemExit("no published articles available")
    ids = [article["id"] for article in articles]
    rng = random.Random(2026)
    counts = Counter()
    started = time.monotonic()
    for index in range(args.requests):
        kind = index % 8
        article_id = rng.choice(ids)
        if kind == 0:
            path = "/articles?" + urllib.parse.urlencode({"page": 1 + (index // 8) % 3, "pageSize": 10})
        elif kind == 1:
            path = f"/articles/{article_id}"
        elif kind == 2:
            path = f"/articles/{article_id}/comments"
        elif kind == 3:
            path = "/articles?" + urllib.parse.urlencode({"q": rng.choice(["数据库", "写作", "观察", "阅读"])})
        elif kind == 4:
            path = "/articles?" + urllib.parse.urlencode({"tag": rng.choice(["可观测性", "数据库", "生活记录", "摄影"])})
        elif kind == 5:
            path = "/tags"
        elif kind == 6:
            path = f"/articles/{article_id}"
        else:
            path = "/articles/99999999"
        status, _ = request(base + path)
        counts[status] += 1
        if status not in (200, 404):
            raise SystemExit(f"unexpected HTTP {status} for {path}")
        if index + 1 < args.requests:
            time.sleep(args.interval)
    elapsed = time.monotonic() - started
    print(f"Sent {args.requests} requests in {elapsed:.1f}s: " + ", ".join(f"HTTP {code}={count}" for code, count in sorted(counts.items())))


if __name__ == "__main__":
    main()
