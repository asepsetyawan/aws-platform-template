"""Install monitoring only after operator-created credential/alert secrets exist."""
import argparse
import subprocess
from pathlib import Path
from download_charts import ROOT, download

parser = argparse.ArgumentParser()
parser.add_argument("--context", required=True)
parser.add_argument("--region", required=True)
parser.add_argument("--loki-bucket", required=True)
parser.add_argument("--tempo-bucket", required=True)
args = parser.parse_args()


def run(*command):
    subprocess.run(command, check=True)


for secret in ["grafana-admin", "platform-alertmanager", "oncall-webhook"]:
    run("kubectl", "--context", args.context, "-n", "observability", "get", "secret", secret)
download()
destination = ROOT / ".generated/observability"
destination.mkdir(parents=True, exist_ok=True)
for release, chart in [("loki", "loki"), ("tempo", "tempo"), ("otel", "opentelemetry-collector"), ("alloy", "alloy"), ("monitoring", "kube-prometheus-stack")]:
    content = (ROOT / f"observability/helm/{release}.yaml").read_text()
    content = content.replace("REPLACE_LOKI_BUCKET", args.loki_bucket).replace("REPLACE_TEMPO_BUCKET", args.tempo_bucket).replace("ap-southeast-1", args.region)
    values = destination / f"{release}.yaml"
    values.write_text(content)
    run("helm", "upgrade", "--install", release, str(ROOT / f".generated/charts/{chart}.tgz"),
        "--kube-context", args.context, "--namespace", "observability", "--wait", "--timeout", "20m", "-f", str(values))
run("kubectl", "--context", args.context, "apply", "-f", str(ROOT / "observability/manifests"))
# ConfigMap is generated locally, then applied to the selected cluster.
result = subprocess.run(["kubectl", "create", "configmap", "api-dashboard", "--namespace", "observability",
                         "--from-file=" + str(ROOT / "observability/dashboards/api.json"), "--dry-run=client", "-o", "yaml"],
                        check=True, capture_output=True, text=True)
subprocess.run(["kubectl", "--context", args.context, "apply", "-f", "-"], input=result.stdout, text=True, check=True)
run("kubectl", "--context", args.context, "-n", "observability", "label", "configmap", "api-dashboard", "grafana_dashboard=1", "--overwrite")
