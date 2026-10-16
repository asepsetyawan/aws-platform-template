# PostgreSQL recovery

The sample API is stateless; RDS in staging/prod is a provisioned extension point. Add a least-privilege application database user and migrations before use. Never give the application the RDS master secret.

1. Identify incident type: connection failure, zone failover, logical corruption, deletion, or regional outage.
2. For failover, confirm RDS events and retry behavior; Multi-AZ standby is not a read replica.
3. For corruption, select a clean restore point. PITR creates a new instance; restore in an isolated recovery environment using the correct subnet/security group/KMS configuration.
4. Validate integrity, migrations, permissions, and application behavior before switching endpoints through reviewed configuration.
5. Record measured RTO/RPO, including investigation, restore, validation, and traffic switch time.
6. Reconcile restored resources with Terraform through a reviewed import/configuration plan. Do not copy old Terraform state blindly.

Cross-region backup replication/copy and a second-region runtime are not included in the default modules. Define destination-region KMS/IAM/backup policies and perform regional restore drills before claiming regional disaster recovery.

Production deletion protection must be intentionally disabled before approved decommissioning. Final snapshot name must be unique for repeated recreate/destroy cycles; change it before decommissioning if an older final snapshot already exists.
