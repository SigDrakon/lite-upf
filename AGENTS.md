# AGENTS.md - Technical Notes for lite-upf

This file contains project-specific guidance for AI coding agents and future development sessions. Treat it as the operational companion to `README.md`: the README can describe the project to users; this file records implementation direction, engineering constraints, conventions, and common pitfalls for agents modifying the codebase.

The repository is currently minimal, so this document intentionally defines a starting architecture and working assumptions rather than documenting a mature implementation.

## Project Overview

`lite-upf` is intended to be a lightweight User Plane Function (UPF) implementation or UPF-oriented runtime component.

The likely scope is narrower than a production-grade 5G core UPF. Prioritize clarity, testability, and incremental protocol support over broad feature coverage.

Core goals:

- Implement a small, understandable UPF data-plane/control-plane core.
- Support PFCP-driven session and forwarding rule management where applicable.
- Support GTP-U packet handling on the N3 side.
- Provide deterministic behavior suitable for lab validation and future automated tests.
- Keep dependencies and runtime assumptions minimal.

## Repository Status

At the time this file was added, the repository only contained a minimal `README.md`. Do not assume existing package layout, build system, or runtime architecture until files are added.

When creating the initial implementation, prefer a small structure that can grow without becoming obscure.

Suggested initial layout if using Go:

```text
cmd/lite-upf/          CLI entry point
internal/config/      configuration loading and validation
internal/pfcp/        PFCP server/session/rule handling
internal/gtpu/        GTP-U socket, parsing, encapsulation
internal/session/     session, PDR/FAR/QER/URR state
internal/forwarder/   packet classification and forwarding path
internal/logging/     logging helpers if needed
configs/              example runtime configs
tests/ or testdata/   fixtures and integration test assets
```

Adjust this structure only when the implementation demands it.

## Design Principles

### Keep the Core Small

This project should remain a lite UPF. Avoid importing large frameworks or implementing broad 3GPP feature sets before the core forwarding path is reliable.

Prefer this order:

1. Configuration and process lifecycle.
2. GTP-U receive/decode path.
3. Session state model.
4. Minimal PFCP association/session establishment support.
5. Packet classification using PDR-like rules.
6. FAR-like forwarding behavior.
7. Observability and test harnesses.

### Make Protocol State Explicit

UPF behavior is stateful. Keep protocol state explicit in named structs rather than hidden behind generic maps.

Important concepts to model clearly:

- PFCP node association state.
- Local and remote SEIDs.
- PDRs: packet detection rules.
- FARs: forwarding action rules.
- TEIDs and UE IP mappings.
- N3, N4, and N6 interface addresses.
- Session lifecycle: establish, modify, delete.

### Separate Control Plane and Data Plane

Do not mix PFCP message handling directly into packet forwarding code.

Recommended separation:

- PFCP/control-plane code mutates session/rule state.
- Data-plane code reads session/rule state to classify and forward packets.
- Shared state should have a clear ownership and synchronization strategy.

### Prefer Deterministic Behavior

The implementation should be suitable for tests and packet-level debugging.

- Avoid hidden global mutable state.
- Make allocators deterministic where possible.
- Make config validation strict.
- Make logs include stable identifiers such as SEID, TEID, UE IP, PDR ID, and FAR ID.

## Configuration Conventions

Use configuration for all lab-specific values.

Do not hard-code:

- N4 listen address.
- N3 listen address.
- N6 interface or next-hop address.
- UE IP pools.
- MTU assumptions.
- Log level.

A future config file might include:

```yaml
n4:
  listen: "0.0.0.0:8805"
n3:
  listen: "0.0.0.0:2152"
n6:
  interface: "eth0"
logging:
  level: "info"
features:
  pfcp: true
  gtpu: true
```

When adding configuration fields, add validation immediately. Missing or malformed network settings should fail at startup with actionable errors.

## Protocol Implementation Notes

### PFCP

Implement PFCP incrementally.

Suggested minimum sequence:

1. Association Setup Request/Response.
2. Session Establishment Request/Response.
3. Session Modification Request/Response.
4. Session Deletion Request/Response.
5. Heartbeat support if needed for compatibility.

Keep IE parsing and semantic validation separate:

- Parsing should decode message structure.
- Validation should check required IEs and project-supported constraints.
- Application should update session/rule state.

Do not silently accept unsupported mandatory behavior. Return explicit cause values or errors when a request cannot be honored.

### GTP-U

The GTP-U path should be easy to inspect.

At minimum, keep these steps distinct:

1. Read UDP packet from N3 socket.
2. Parse GTP-U header.
3. Extract TEID.
4. Look up session/rule state.
5. Decapsulate payload.
6. Forward, drop, or buffer based on configured action.
7. Update counters/logs.

For outbound/downlink support, define the reverse path just as explicitly:

1. Receive or inject plain IP packet from N6/test harness.
2. Match UE IP/session.
3. Encapsulate with GTP-U using session TEID and peer address.
4. Send to N3 peer.

### Session and Rule Model

Even if the first implementation is simple, name the rule concepts after UPF/PFCP terminology where practical:

- `Session`
- `PDR`
- `FAR`
- `QER`
- `URR`
- `TEID`
- `SEID`

This makes future protocol growth easier and keeps code aligned with UPF domain language.

## Error Handling Philosophy

- Fail fast on invalid configuration.
- Reject unsupported PFCP requests explicitly.
- Treat malformed packets as packet-level errors, not process-level fatal errors.
- Do not let one bad packet terminate the UPF process.
- Do not ignore socket errors; classify them as transient or fatal.
- Preserve enough context in errors to debug packet/session behavior.

Good error context includes:

- Local and remote endpoint.
- Message type.
- SEID or TEID.
- Session ID if defined.
- PDR/FAR ID if relevant.

## Logging and Observability

Logs should help debug protocol and forwarding behavior without requiring a debugger.

Useful log fields:

- Event type: startup, PFCP request, PFCP response, packet received, packet forwarded, packet dropped.
- Interface: N3, N4, N6.
- Remote/local endpoint.
- SEID.
- TEID.
- UE IP.
- Rule IDs.
- Cause or drop reason.

Avoid logging full packet payloads by default. Add a debug flag before printing raw bytes.

Future useful metrics:

- Packets received on N3.
- Packets forwarded to N6.
- Packets encapsulated to N3.
- Packet drops by reason.
- Active sessions.
- Active PDR/FAR counts.
- PFCP requests by type and result.

## Testing Strategy

Prioritize small tests before integration tests.

Suggested test layers:

1. Unit tests for config validation.
2. Unit tests for GTP-U header parse/encode.
3. Unit tests for TEID/session lookup.
4. Unit tests for PFCP message handling where feasible.
5. Table-driven tests for rule matching.
6. Integration tests using a local UDP harness.
7. Lab tests against a known SMF/UPF tester setup.

If using Go, run where applicable:

```bash
go fmt ./...
go test ./...
go vet ./...
```

Do not add tests that require root privileges, specific NIC names, or lab IPs unless they are clearly marked as integration/manual tests.

## Common Gotchas

1. **N3/N4/N6 confusion**
   N4 is PFCP control plane. N3 is GTP-U access side. N6 is data network side. Keep them separate in config, logs, and code.

2. **TEID directionality**
   Uplink and downlink TEIDs may not be interchangeable. Be explicit about which peer allocated which identifier.

3. **SEID directionality**
   Local SEID and remote SEID have different meanings. Store both with clear field names.

4. **Global session maps**
   A global map may be acceptable for the first prototype, but define synchronization early if PFCP and data-plane goroutines access it concurrently.

5. **Silent packet drops**
   Every intentional drop path should have a reason counter and optional debug log.

6. **Checksum and MTU issues**
   Data-plane bugs may appear as remote connectivity failures. Keep packet encoding, checksum behavior, and MTU assumptions inspectable.

7. **Privilege assumptions**
   Raw sockets, TUN/TAP devices, and interface manipulation may need elevated privileges. Keep privileged operations isolated and documented.

## Agent Workflow

When making changes as an AI coding agent:

1. Inspect current repository contents before designing architecture.
2. Prefer minimal, reviewable patches.
3. Do not invent broad production requirements without user confirmation.
4. Preserve clear separation between PFCP, GTP-U, session state, and forwarding.
5. Add tests with new protocol parsing or rule-matching logic.
6. Update `README.md` when user-facing commands or architecture become concrete.
7. Update this file when project conventions or architecture decisions change.
8. Report exactly what changed and what was not tested.

## Initial Implementation Roadmap

A reasonable first milestone:

```text
config loader
    ↓
N3 UDP socket listener
    ↓
GTP-U header parser
    ↓
in-memory session table
    ↓
manual/static TEID-to-UE mapping
    ↓
packet classification logs
```

A reasonable second milestone:

```text
PFCP Association Setup
    ↓
PFCP Session Establishment
    ↓
PDR/FAR extraction
    ↓
dynamic session table updates
    ↓
GTP-U forwarding decisions based on PFCP state
```

A reasonable third milestone:

```text
N6 forwarding path
    ↓
downlink encapsulation
    ↓
packet/drop counters
    ↓
integration tests
    ↓
README usage examples
```

## Future Enhancement Ideas

Do not implement these opportunistically unless requested:

- TUN/TAP-based N6 integration.
- eBPF or AF_XDP acceleration.
- Prometheus metrics endpoint.
- pcap dump support.
- PFCP heartbeat and recovery timestamp handling.
- QER/URR enforcement.
- Multi-session load testing.
- CI integration with packet-level unit tests.
- Compatibility profiles for free5GC or Open5GS.
