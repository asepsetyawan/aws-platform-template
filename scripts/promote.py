"""Promote exactly one service image in one environment. Does not call Git or AWS."""
import re
import sys
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parents[1]


def promote(environment: str, service: str, repository: str, digest: str, root: Path = ROOT):
    if environment not in {"dev", "staging", "prod"}:
        raise ValueError("environment must be dev, staging or prod")
    if service not in {"api", "catalog"}:
        raise ValueError("service must be api or catalog")
    if not re.fullmatch(r"[0-9]{12}\.dkr\.ecr\.[a-z0-9-]+\.amazonaws\.com/[a-z0-9/_-]+", repository):
        raise ValueError("expected a commercial AWS ECR repository without a tag")
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", digest) or digest == "sha256:" + "0" * 64:
        raise ValueError("expected a non-placeholder sha256 digest")
    path = root / "gitops/apps/overlays" / environment / "kustomization.yaml"
    document = yaml.safe_load(path.read_text())
    images = [image for image in document["images"] if image["name"] == f"platform-{service}"]
    if len(images) != 1:
        raise ValueError("expected exactly one matching service image")
    images[0]["newName"], images[0]["digest"] = repository, digest
    path.write_text(yaml.safe_dump(document, sort_keys=False))


if __name__ == "__main__":
    promote(*sys.argv[1:])
