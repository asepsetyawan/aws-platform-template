"""Fail before application bootstrap if personalized references remain placeholders."""
import sys
from pathlib import Path

root = Path(__file__).resolve().parents[1]
environment = sys.argv[1]
if environment not in {"dev", "staging", "prod"}:
    raise SystemExit("Use dev, staging, or prod")
files = list((root / f"gitops/apps/overlays/{environment}").glob("*.yaml"))
files += list((root / f"gitops/argocd/{environment}").glob("*.yaml"))
for path in files:
    text = path.read_text()
    if "REPLACE_" in text or "sha256:" + "0" * 64 in text:
        raise SystemExit(f"Personalize {path.relative_to(root)} before deployment")
print("Application references personalized; validate permissions, image provenance, and target cluster separately")
