"""Docker HEALTHCHECK для helper/ — python:3.12-slim, в отличие от
distroless backend/, всё же несёт полноценный интерпретатор, поэтому
отдельного режима в app/main.py не требуется (в отличие от
backend/cmd/api/healthcheck.go): маленький самостоятельный скрипт проще,
чем протаскивать shell/curl в образ ради одной проверки."""

import os
import sys
import urllib.request

if __name__ == "__main__":
    port = os.environ.get("HEALTH_HTTP_PORT", "8001")
    try:
        with urllib.request.urlopen(f"http://localhost:{port}/healthz", timeout=2) as resp:
            sys.exit(0 if resp.status == 200 else 1)
    except OSError:
        sys.exit(1)
