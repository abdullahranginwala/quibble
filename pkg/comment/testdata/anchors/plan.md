# Deployment Plan

This document covers the rollout of the new payment charge flow across all regions.

## Rollback plan

### Failure modes

The retry loop will re-attempt every 30 minutes until the charge succeeds or the booking is cancelled.

Guest is charged but marked failed, which opens a double-charge window that must be closed by reconciliation.

### Recovery

Operators can trigger a manual reconciliation from the admin console at any time during an incident.

## Monitoring

Dashboards track charge success rate, latency, and the daily reconciliation count across every currency.

## Appendix

Refer to the runbook for the full escalation matrix and on-call rotation details.
