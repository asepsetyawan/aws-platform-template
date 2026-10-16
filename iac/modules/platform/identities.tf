locals {
  identities = {
    "grafana"        = { namespace = "observability", service_account = "grafana" }
    "alb-controller" = { namespace = "kube-system", service_account = "aws-load-balancer-controller" }
    "autoscaler"     = { namespace = "kube-system", service_account = "cluster-autoscaler" }
    "ebs-csi"        = { namespace = "kube-system", service_account = "ebs-csi-controller-sa" }
    "loki"           = { namespace = "observability", service_account = "loki" }
    "tempo"          = { namespace = "observability", service_account = "tempo" }
  }
}
resource "aws_iam_role" "workload" {
  for_each = local.identities
  name     = "${var.name}-${each.key}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow", Principal = { Service = "pods.eks.amazonaws.com" }
      Action = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })
}
resource "aws_eks_pod_identity_association" "workload" {
  # EBS CSI association is owned by its EKS addon instead.
  for_each        = { for k, v in local.identities : k => v if k != "ebs-csi" }
  cluster_name    = aws_eks_cluster.this.name
  namespace       = each.value.namespace
  service_account = each.value.service_account
  role_arn        = aws_iam_role.workload[each.key].arn
  depends_on      = [aws_eks_addon.agent]
}
resource "aws_iam_role_policy_attachment" "ebs" {
  role       = aws_iam_role.workload["ebs-csi"].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
}
resource "aws_iam_role_policy" "alb" {
  role   = aws_iam_role.workload["alb-controller"].name
  policy = file("${path.module}/policies/alb-controller.json")
}
resource "aws_iam_role_policy" "telemetry" {
  for_each = toset(["loki", "tempo"])
  role     = aws_iam_role.workload[each.key].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["kms:Decrypt", "kms:GenerateDataKey"], Resource = aws_kms_key.platform.arn },
      { Effect = "Allow", Action = ["s3:ListBucket"], Resource = aws_s3_bucket.telemetry[each.key].arn },
      { Effect = "Allow", Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"], Resource = "${aws_s3_bucket.telemetry[each.key].arn}/*" }
    ]
  })
}
resource "aws_iam_role_policy" "autoscaler" {
  role = aws_iam_role.workload["autoscaler"].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["autoscaling:DescribeAutoScalingGroups", "autoscaling:DescribeAutoScalingInstances", "autoscaling:DescribeLaunchConfigurations", "autoscaling:DescribeScalingActivities", "autoscaling:DescribeTags", "ec2:DescribeImages", "ec2:DescribeInstanceTypes", "ec2:DescribeLaunchTemplateVersions", "ec2:GetInstanceTypesFromInstanceRequirements", "eks:DescribeNodegroup"], Resource = "*" },
      { Effect = "Allow", Action = ["autoscaling:SetDesiredCapacity", "autoscaling:TerminateInstanceInAutoScalingGroup"], Resource = "*", Condition = { StringEquals = {
        "aws:ResourceTag/k8s.io/cluster-autoscaler/enabled"     = "true"
        "aws:ResourceTag/k8s.io/cluster-autoscaler/${var.name}" = "owned"
      } } }
    ]
  })
}

resource "aws_iam_role_policy" "grafana" {
  role = aws_iam_role.workload["grafana"].name
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      { Effect = "Allow", Action = ["cloudwatch:ListMetrics", "cloudwatch:GetMetricData", "cloudwatch:GetMetricStatistics", "cloudwatch:DescribeAlarms", "logs:DescribeLogGroups"], Resource = "*" },
      { Effect = "Allow", Action = ["logs:GetLogGroupFields", "logs:StartQuery", "logs:GetQueryResults", "logs:StopQuery"], Resource = ["${aws_cloudwatch_log_group.eks.arn}:*", "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/rds/instance/${var.name}-postgres/*"] }
    ]
  })
}
