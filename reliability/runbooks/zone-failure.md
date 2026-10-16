# Availability-zone failure drill

Run in staging first with an agreed incident owner and recovery criteria.

1. Verify three-AZ node capacity and topology spread before injecting failure. Confirm remaining zones can sustain expected demand.
2. Simulate controlled node disruption in one zone using an approved AWS FIS experiment or staged node drain. PDB governs voluntary evictions and does not prevent an actual zone outage.
3. Observe ALB target health, readiness, pending pods, error ratio, HPA, autoscaler, NAT egress, and database failover when enabled.
4. Confirm alerts reach the actual on-call receiver, not just the Alertmanager UI.
5. Restore capacity and measure recovery time/user impact against agreed objectives. Record whether topology constraints blocked recovery and whether subnet/ASG capacity was sufficient.

A managed node group spanning AZs does not guarantee spare capacity in each zone. Validate placement and reserved/headroom capacity as part of readiness review.
