import argparse
import subprocess
from pathlib import Path
import yaml
from download_charts import ROOT, download

parser = argparse.ArgumentParser()
parser.add_argument("--helm", default="helm")
parser.add_argument("--charts-dir", type=Path, default=ROOT / ".generated/charts")
args = parser.parse_args()
if args.charts_dir == ROOT / ".generated/charts":
    download()

charts = [
    ("argocd", "argo-cd", "platform/argocd.yaml", "argocd", []),
    ("aws-load-balancer-controller", "aws-load-balancer-controller", "platform/alb-controller.yaml", "kube-system", ["--set", "clusterName=aws-template-dev,region=ap-southeast-1,vpcId=vpc-0123456789abcdef0"]),
    ("metrics-server", "metrics-server", "platform/metrics-server.yaml", "kube-system", []),
    ("cluster-autoscaler", "cluster-autoscaler", "platform/autoscaler.yaml", "kube-system", ["--set", "autoDiscovery.clusterName=aws-template-dev,awsRegion=ap-southeast-1,image.tag=v1.35.0"]),
    ("monitoring", "kube-prometheus-stack", "observability/helm/monitoring.yaml", "observability", []),
    ("loki", "loki", "observability/helm/loki.yaml", "observability", []),
    ("tempo", "tempo", "observability/helm/tempo.yaml", "observability", []),
    ("otel", "opentelemetry-collector", "observability/helm/otel.yaml", "observability", []),
    ("alloy", "alloy", "observability/helm/alloy.yaml", "observability", []),
]
for release, chart, values, namespace, extra in charts:
    output = subprocess.check_output([args.helm, "template", release, str(args.charts_dir / f"{chart}.tgz"),
                                      "-n", namespace, "-f", str(ROOT / values), *extra], text=True)
    docs = [d for d in yaml.safe_load_all(output) if d]
    assert docs, chart
    if chart == "tempo":
        service = next(d for d in docs if d["kind"] == "Service" and d["metadata"]["name"] == "tempo")
        assert 3200 in [port["port"] for port in service["spec"]["ports"]]
    if chart == "loki":
        workloads = [d for d in docs if d["kind"] == "StatefulSet" and d["spec"].get("replicas") == 3]
        assert workloads, "Loki must actually render three replicas"
    print(f"Rendered {chart}: {len(docs)} objects")
