# AGENTS.md - Technical Notes for lite-upf

This file contains technical details, architectural decisions, and important implementation notes for future development sessions.

It is adapted from the style and structure of Karpathy's `CLAUDE.md`: a compact project memory file for coding agents. The content below is rewritten for `lite-upf`, not copied as an LLM Council implementation.

## Project Overview

`lite-upf` is a lightweight User Plane Function (UPF) project. The intended design is a small, inspectable UPF implementation focused on PFCP control-plane state, GTP-U data-plane handling, and deterministic lab/debug behavior.

The key innovation should be simplicity: keep the UPF small enough that packet flow, session state, and forwarding decisions are easy to inspect and modify.

## Architecture

The repository is currently minimal, so the architecture below records the intended structure for future implementation.

### Command Structure (`cmd/`)

**`cmd/lite-upf/main.go`**
- Program entry point.
- Loads configuration.
- Initializes logging.
- Starts the PFCP/N4 control-plane listener.
- Starts the GTP-U/N3 data-plane listener.
- Handles graceful shutdown.

The binary should be runnable from the repository root without hard-coded lab paths.

### Configuration Structure (`internal/config/` or `config/`)

**`config.yaml`**
- Contains N4 listen address for PFCP.
- Contains N3 listen address for GTP-U.
- Contains N6 interface or forwarding target configuration.
- Contains log level and debug options.
- Contains optional static sessions for early bring-up before PFCP is complete.

Example shape:

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

Configuration must be validated at startup. Missing or malformed interface/IP/port settings should fail fast with actionable errors.

### PFCP Structure (`internal/pfcp/`)

**`server.go`**
- Owns the UDP PFCP listener on N4.
- Decodes incoming PFCP messages.
- Dispatches messages to handlers by type.
- Encodes and sends PFCP responses.

**`association.go`**
- Handles PFCP Association Setup/Release.
- Tracks peer node identity and recovery timestamp where supported.
- Should keep association state explicit rather than implicit in sessions.

**`session.go`**
- Handles Session Establishment, Modification, and Deletion.
- Extracts or updates PDR/FAR/QER/URR-like rule state as support is added.
- Creates mapping between SEID, TEID, UE IP, and peer endpoint.

**`ie.go` / `parser.go`**
- Decodes PFCP Information Elements.
- Keeps low-level parse logic separate from semantic validation.
- Should not silently discard unsupported mandatory fields.

### GTP-U Structure (`internal/gtpu/`)

**`server.go`**
- Owns the UDP GTP-U listener on N3.
- Reads datagrams and passes them to packet parsing.
- Should keep one bad packet from crashing the process.

**`packet.go`**
- Parses and encodes GTP-U headers.
- Extracts TEID and payload.
- Keeps header encode/decode table-testable.

**`encap.go` / `decap.go`**
- Handles GTP-U encapsulation and decapsulation.
- Should keep uplink and downlink directions explicit.

### Session State Structure (`internal/session/`)

**`store.go`**
- In-memory session table.
- Lookup by local SEID, remote SEID, TEID, and UE IP where applicable.
- Must be concurrency-safe if PFCP and GTP-U run in separate goroutines.

**`session.go`**
- Defines session state.
- Stores local/remote SEIDs distinctly.
- Stores PDR/FAR-like forwarding rules.
- Stores tunnel peer information and UE addressing.

Important: do not collapse local SEID, remote SEID, uplink TEID, and downlink TEID into ambiguous fields. Directionality matters.

### Forwarding Structure (`internal/forwarder/`)

**`classifier.go`**
- Maps packets to session/rule state.
- Uses TEID for uplink GTP-U traffic.
- Uses UE IP or rule state for downlink traffic when supported.

**`forwarder.go`**
- Applies FAR-like forwarding actions.
- Handles forward/drop/buffer behavior as implemented.
- Emits structured drop reasons.

**`n6.go`**
- Encapsulates N6-side behavior.
- May begin as a test harness or UDP/TUN abstraction before full interface integration.

### Utility Structure (`internal/util/`)

**`id.go`**
- Helpers for SEID, TEID, and rule IDs if needed.

**`net.go`**
- Address parsing and endpoint helpers.

**`log.go`**
- Logging helpers if the standard logger becomes insufficient.

## Key Design Decisions

### Control/Data Plane Separation

PFCP code should not directly forward packets. GTP-U code should not directly mutate PFCP session semantics.

- PFCP handlers update session/rule state.
- GTP-U handlers read session/rule state and classify packets.
- Forwarding code applies actions based on that state.

This separation keeps the implementation debuggable and prevents protocol state from leaking into packet I/O code.

### Explicit Directionality

UPF code is easy to break if identifiers are treated as interchangeable.

Keep these concepts separate:

- Local SEID vs remote SEID.
- Uplink TEID vs downlink TEID.
- N3 peer address vs N6 forwarding target.
- PFCP peer state vs user-plane session state.

Field names should encode direction or ownership where possible, for example `LocalSEID`, `RemoteSEID`, `UplinkTEID`, `DownlinkTEID`.

### Incremental Protocol Support

Do not attempt a full production UPF in one pass. Implement a narrow, testable path first:

1. Config loading and process lifecycle.
2. GTP-U packet parse/encode tests.
3. Static TEID/session mapping.
4. N3 receive path with classification logs.
5. PFCP Association Setup.
6. PFCP Session Establishment.
7. Dynamic PDR/FAR extraction.
8. Forward/drop actions.
9. N6 path and downlink encapsulation.

### Error Handling Philosophy

- Fail fast on invalid configuration.
- Reject unsupported PFCP requests explicitly.
- Treat malformed packets as packet-level errors, not process-level fatal errors.
- Do not silently ignore socket errors.
- Do not silently drop packets without a reason counter or debug log.
- Preserve context in errors: endpoint, message type, SEID, TEID, UE IP, PDR ID, FAR ID.

### Observability

The project should be easy to debug in a packet lab.

Logs should identify:

- Interface: N3, N4, or N6.
- Event type: startup, PFCP request/response, packet received, packet forwarded, packet dropped.
- Remote/local endpoint.
- SEID and TEID.
- UE IP.
- Rule IDs.
- Cause or drop reason.

Avoid full payload logging by default. Raw packet dumps should require an explicit debug option.

## Important Implementation Details

### Repository Status

At the time this file was written, `lite-upf` only had a minimal `README.md`. Do not assume existing package layout, build commands, or runtime conventions until files exist.

### Suggested Initial Layout

```text
cmd/lite-upf/          CLI entry point
internal/config/      configuration loading and validation
internal/pfcp/        PFCP server/session/rule handling
internal/gtpu/        GTP-U socket, parsing, encapsulation
internal/session/     session, PDR/FAR/QER/URR state
internal/forwarder/   packet classification and forwarding path
configs/              example runtime configs
testdata/             packet fixtures and config fixtures
```

### Concurrency

PFCP and GTP-U paths will likely run concurrently. Session/rule state must have an explicit synchronization strategy.

Acceptable early options:

- A single `SessionStore` guarded by `sync.RWMutex`.
- A serialized control loop that owns state and receives update/query messages.

Do not use unsynchronized global maps across packet and PFCP goroutines.

### Configuration

All lab-specific values must come from configuration:

- N4 listen address.
- N3 listen address.
- N6 interface or forwarding target.
- UE IP pool or static UE mappings.
- MTU assumptions.
- Log level.

Avoid absolute paths and host-specific interface names in code.

### Testing

If using Go, the normal validation commands should be:

```bash
go fmt ./...
go test ./...
go vet ./...
```

Packet and protocol logic should be table-tested where possible. Integration tests that require root privileges, TUN/TAP, or specific NIC names must be clearly marked as manual/integration tests.

## Common Gotchas

1. **N3/N4/N6 confusion**: N4 is PFCP control plane, N3 is GTP-U access side, and N6 is data network side. Keep them separate in config, logs, and code.

2. **SEID directionality**: Local and remote SEIDs are not interchangeable. Bugs here break modification and deletion.

3. **TEID directionality**: Uplink and downlink TEIDs may have different owners and uses. Store them with explicit names.

4. **Global session maps**: A global map might work in a prototype, but unsynchronized access will fail once PFCP and GTP-U paths are concurrent.

5. **Silent packet drops**: Every intentional drop path needs a reason. Counters are preferable; debug logs are acceptable early on.

6. **Privilege assumptions**: Raw sockets, TUN/TAP, and interface operations may require elevated privileges. Keep privileged code isolated and documented.

7. **MTU/checksum issues**: Connectivity failures may come from packet formatting, MTU, routing, checksum, or TEID mismatch. Do not assume the UPF rule lookup is the only possible fault.

## Future Enhancement Ideas

- Static-session mode for early GTP-U testing without PFCP.
- PFCP Heartbeat support.
- PDR/FAR extraction from real Session Establishment messages.
- QER/URR support.
- N6 integration through TUN/TAP.
- Prometheus metrics endpoint.
- pcap dump support.
- Structured JSON logs.
- Compatibility profile for free5GC or Open5GS.
- Packet-level integration tests with local UDP harnesses.
- CI workflow for `go fmt`, `go test`, and packet parser tests.

## Testing Notes

Start with deterministic local tests before lab integration:

- GTP-U header parse/encode tests.
- Session store lookup tests.
- Config validation tests.
- Rule matching table tests.
- PFCP handler tests using encoded fixture messages when available.

Only after these pass should lab testing against an SMF or external UPF tester be used as the primary validation path.

## Data Flow Summary

```text
PFCP Session Establishment
    ↓
Decode PFCP IEs
    ↓
Validate supported PDR/FAR/session fields
    ↓
Update session store
    ↓
GTP-U packet arrives on N3
    ↓
Parse GTP-U header and TEID
    ↓
Lookup session/rule state
    ↓
Apply forwarding action
    ↓
Forward/drop/log/counter update
```

For downlink support:

```text
Plain IP packet from N6/test harness
    ↓
Match UE IP/session/rule
    ↓
Select tunnel peer and TEID
    ↓
Encapsulate as GTP-U
    ↓
Send on N3
```

Keep the entire path explicit and inspectable. That is the main value of `lite-upf`.
