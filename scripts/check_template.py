"""Check two-service Kubernetes/CI contracts without contacting AWS or Kubernetes."""
import json
import re
import subprocess
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parents[1]
for environment in ("dev", "staging", "prod"):
    output = subprocess.check_output(["kubectl", "kustomize", str(ROOT / f"gitops/apps/overlays/{environment}")], text=True)
    docs = {(d["kind"], d["metadata"]["name"]): d for d in yaml.safe_load_all(output)}
    namespace = "platform-" + environment
    for name in ("api", "catalog"):
        deployment = docs["Deployment", name]
        service = docs["Service", name]
        pod = deployment["spec"]["template"]
        assert deployment["metadata"]["namespace"] == namespace
        assert "replicas" not in deployment["spec"], "HPA must own scaling"
        assert service["spec"]["selector"] == deployment["spec"]["selector"]["matchLabels"] == pod["metadata"]["labels"]
        hpa = docs["HorizontalPodAutoscaler", name]
        assert hpa["spec"]["scaleTargetRef"]["name"] == name
        assert docs["PodDisruptionBudget", name]["spec"]["maxUnavailable"] < hpa["spec"]["minReplicas"]
        container = pod["spec"]["containers"][0]
        port_names = {port["name"] for port in container["ports"]}
        assert all(port["targetPort"] in port_names for port in service["spec"]["ports"])
        assert container["readinessProbe"]["httpGet"]["path"] == "/readyz"
        assert container["livenessProbe"]["httpGet"]["path"] == "/livez"
        assert container["securityContext"]["readOnlyRootFilesystem"]
        assert container["lifecycle"]["preStop"]["exec"]["command"] == ["/server", "drain"]
        assert len(pod["spec"]["topologySpreadConstraints"]) >= 2
        env = {item["name"]: item["value"] for item in container["env"]}
        assert "GRPC_INSECURE" not in env, "Kubernetes must use verified gRPC TLS"
        assert env["APP_ENV"] == environment
    catalog_service = docs["Service", "catalog"]
    assert catalog_service["spec"]["type"] == "ClusterIP" and catalog_service["spec"]["clusterIP"] == "None"
    assert docs["Ingress", "api"]["metadata"]["annotations"]["alb.ingress.kubernetes.io/scheme"] == "internal"
    assert ("Ingress", "catalog") not in docs
    for rule in docs["Ingress", "api"]["spec"]["rules"]:
        assert all(path["backend"]["service"]["name"] == "kong" for path in rule["http"]["paths"])
    kong = docs["Deployment", "kong"]
    container = kong["spec"]["template"]["spec"]["containers"][0]
    assert "replicas" not in kong["spec"]
    assert container["readinessProbe"]["httpGet"] == {"path": "/status/ready", "port": "status"}
    env = {item["name"]: item["value"] for item in container["env"]}
    assert env["KONG_DATABASE"] == "off" and env["KONG_ADMIN_LISTEN"] == "off"
    assert env["KONG_ADMIN_GUI_LISTEN"] == "off"
    hpa = docs["HorizontalPodAutoscaler", "kong"]["spec"]
    assert hpa["minReplicas"] == (2 if environment == "dev" else 3)
    assert docs["PodDisruptionBudget", "kong"]["spec"]["maxUnavailable"] < hpa["minReplicas"]
    config_name = kong["spec"]["template"]["spec"]["volumes"][1]["configMap"]["name"]
    assert config_name.startswith("kong-config-")  # Config change triggers rollout.
    config = yaml.safe_load(docs["ConfigMap", config_name]["data"]["kong.yaml"])
    service = config["services"][0]
    assert service["url"] == "http://api:8080" and service["retries"] == 0
    assert service["routes"][0]["paths"] == ["/api/"] and not service["routes"][0]["strip_path"]
    api_ingress = docs["NetworkPolicy", "api"]["spec"]["ingress"]
    assert all("ipBlock" not in peer for rule in api_ingress for peer in rule["from"])
    assert {"podSelector": {"matchLabels": {"app": "kong"}}} in api_ingress[0]["from"]
    kong_ingress = docs["Ingress", "api"]["metadata"]["annotations"]
    assert kong_ingress["alb.ingress.kubernetes.io/healthcheck-port"] == "8100"
    assert kong_ingress["alb.ingress.kubernetes.io/healthcheck-path"] == "/status/ready"
    catalog_policy = docs["NetworkPolicy", "catalog"]["spec"]
    grpc_rule = next(rule for rule in catalog_policy["ingress"] if any(port["port"] == 50051 for port in rule["ports"]))
    assert grpc_rule["from"] == [{"podSelector": {"matchLabels": {"app": "api"}}}]
    api_egress = docs["NetworkPolicy", "api"]["spec"]["egress"]
    assert any(rule.get("to") == [{"podSelector": {"matchLabels": {"app": "catalog"}}}] and rule["ports"][0]["port"] == 50051 for rule in api_egress)
    for layer in ("platform", "edge"):
        main = ROOT / f"iac/environments/{environment}/{layer}/main.tf"
        for source in re.findall(r'source\s*=\s*"([.][^"]+)"', main.read_text()):
            assert (main.parent / source).resolve().is_dir()
for workflow in (ROOT / ".github/workflows").glob("*.yml"):
    content = workflow.read_text()
    yaml.safe_load(content)
    assert "pull_request_target" not in content
    for ref in re.findall(r"uses:\s*[^@\s]+@([^\s]+)", content):
        assert re.fullmatch(r"[a-f0-9]{40}", ref), "Actions must be pinned to commit SHA"
json.loads((ROOT / "observability/dashboards/api.json").read_text())
assert yaml.safe_load((ROOT / "observability/rules.yaml").read_text()) == yaml.safe_load((ROOT / "observability/manifests/alerts.yaml").read_text())["spec"]
assert (ROOT / "app/go.sum").is_file()
local = yaml.safe_load((ROOT / "app/kong.yaml").read_text())
assert local["services"] == config["services"]
compose = yaml.safe_load((ROOT / "app/compose.yaml").read_text())["services"]
assert "ports" not in compose["api"] and "ports" not in compose["catalog"]
assert compose["kong"]["image"] == container["image"]
print("Kong gateway, REST/gRPC, TLS, routing, scaling and CI contracts passed for dev/staging/prod")
