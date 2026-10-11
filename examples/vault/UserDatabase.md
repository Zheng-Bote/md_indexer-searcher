---
type: database
name: UserDatabase
engine: postgres
replication: streaming
tier: critical
---
# User Database

Primary relational PostgreSQL cluster holding identity accounts, role mappings, and password hashes.

## Connectivity & Dependents

Accessed directly by [[AuthService]] via connection pooling.
Data is encrypted at rest using envelope encryption.

## Maintenance
- Automated daily backups
- Read-replicas enabled for reporting
