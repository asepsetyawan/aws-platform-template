"""Download only pinned chart URLs and verify SHA256 before Helm execution."""
import hashlib
import urllib.request
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parents[1]


def download():
    destination = ROOT / ".generated/charts"
    destination.mkdir(parents=True, exist_ok=True)
    lock = yaml.safe_load((ROOT / "platform/charts.lock.yaml").read_text())
    for name, chart in lock.items():
        output = destination / f"{name}.tgz"
        if not output.exists() or hashlib.sha256(output.read_bytes()).hexdigest() != chart["sha256"]:
            with urllib.request.urlopen(chart["url"], timeout=60) as response:
                content = response.read()
            if hashlib.sha256(content).hexdigest() != chart["sha256"]:
                raise ValueError(f"checksum mismatch for {name}")
            output.write_bytes(content)
        print(f"Verified {name} {chart['version']}")


if __name__ == "__main__":
    download()
