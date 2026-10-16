"""Run after docker compose up --build --wait; all calls go through Kong."""
import json
import urllib.request
import urllib.error

base = "http://127.0.0.1:8080"

def request(path, expected):
    try:
        response = urllib.request.urlopen(base + path, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        assert response.status == expected, (path, response.status)
        # Proxied responses identify Kong in Via; gateway-generated errors use Server.
        gateway_headers = response.headers.get("Via", "") + " " + response.headers.get("Server", "")
        assert "kong" in gateway_headers.lower(), "Request bypassed Kong"
        return response.read()

assert json.loads(request("/api/products/sku-001", 200))["id"] == "sku-001"
request("/api/hello", 200)
request("/api/products/nonexistent", 404)
request("/metrics", 404)
request("/readyz", 404)
print("Kong → Go REST → gRPC catalog and private endpoint checks passed")
