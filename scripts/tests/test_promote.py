import sys
import tempfile
import unittest
from pathlib import Path
import yaml
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from promote import promote


class PromotionTests(unittest.TestCase):
    def test_only_selected_service_and_environment_changes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            paths = []
            for env in ("dev", "prod"):
                path = root / f"gitops/apps/overlays/{env}/kustomization.yaml"
                path.parent.mkdir(parents=True)
                path.write_text(yaml.safe_dump({"images": [{"name": "platform-api", "newName": "old-api", "digest": "sha256:" + "1" * 64}, {"name": "platform-catalog", "newName": "old-catalog", "digest": "sha256:" + "2" * 64}]}))
                paths.append(path)
            prod_before = paths[1].read_bytes()
            promote("dev", "catalog", "123456789012.dkr.ecr.ap-southeast-1.amazonaws.com/platform/catalog", "sha256:" + "3" * 64, root)
            images = yaml.safe_load(paths[0].read_text())["images"]
            self.assertEqual(images[0]["newName"], "old-api")
            self.assertEqual(images[1]["digest"], "sha256:" + "3" * 64)
            self.assertEqual(paths[1].read_bytes(), prod_before)

    def test_rejects_invalid_inputs_before_writing(self):
        for env, service, repo, digest in [
            ("../prod", "api", "x", "x"),
            ("dev", "../catalog", "x", "x"),
            ("dev", "api", "https://untrusted.invalid", "sha256:" + "3" * 64),
            ("dev", "api", "123456789012.dkr.ecr.ap-southeast-1.amazonaws.com/api", "sha256:" + "0" * 64),
        ]:
            with self.assertRaises(ValueError): promote(env, service, repo, digest)
