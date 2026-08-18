# Security model

KernelQuest treats incident commands as untrusted. Docker provides a practical local-development boundary, not a complete hostile multi-tenant isolation guarantee; a public deployment should move the runtime plane to dedicated hosts or microVMs.

| Threat | Mitigation | Remaining risk |
| --- | --- | --- |
| Sandbox escape or host-file access | No bind mounts, no Docker socket, rootless/non-privileged containers, read-only filesystems where scenarios permit | Kernel vulnerabilities require prompt host patching |
| Fork bombs and resource exhaustion | Per-session CPU, memory, PID, disk-quota, and timeout limits | Limits must be tuned for the runtime host |
| Internal network scanning | Per-session internal network; no host networking; egress policy where supported | Docker network policy is weaker than a dedicated network appliance |
| Cross-user access | UUID session IDs plus authenticated ownership checks for REST and WebSockets | Authorization regression is covered by integration tests |
| WebSocket hijacking | Same authenticated session, origin checks, short-lived WS token, TLS in production | Browser compromise remains outside service control |
| Stale runtimes | Reaper reconciles database state with runtime labels and emits audit events | Host outage can delay cleanup |
| Secret exposure | Do not persist terminal input/output secrets; structured errors suppress stack traces and engine details | Scenario authors must avoid seeding secrets |

Management audit events include `SESSION_CREATED`, `ENVIRONMENT_PROVISIONED`, `RESET_REQUESTED`, `INCIDENT_COMPLETED`, `INCIDENT_EXPIRED`, and `ENVIRONMENT_DESTROYED` with relevant IDs only.

