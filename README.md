# lite-upf

A lightweight 5G **User Plane Function** with a full CI/CD pipeline.

`lite-upf` is a from-scratch, deliberately small UPF focused on PFCP control-plane
state, GTP-U data-plane handling, and deterministic lab/debug behavior. The
design goal is that packet flow, session state, and forwarding decisions stay
easy to inspect and modify.

See [`AGENTS.md`](AGENTS.md) for the architecture, design decisions, and the
incremental implementation plan.

> **Status:** the pipeline is green on `main`. The image is published to
> `ghcr.io/sigdrakon/lite-upf` and is **publicly pullable without a token**
> (the package inherits visibility from this public repository), so
> `docker pull ghcr.io/sigdrakon/lite-upf:latest` works on any host.

---

## Pipeline overview

```
        ┌──────────┐   ┌────────┐   ┌──────────────┐   ┌─────────┐   ┌────────┐
 push ──▶│  Verify  │──▶│  Lint  │──▶│ Build image  │──▶│  GHCR   │──▶│ Deploy │
 PR    ──▶│fmt+vet+  │   │golangci│   │amd64 + arm64 │   │ registry│   │SSH/Compose
        │   test    │   │  lint  │   │              │   │         │   │ +rollback
        └──────────┘   └────────┘   └──────────────┘   └─────────┘   └────────┘
                                            │                                    │
                                     SBOM + provenance                    rollback on
                                                                  failure
```

| Workflow | Trigger | What it does |
|---|---|---|
| `.github/workflows/ci.yml` | push to `main`, tag `v*`, PR, manual | `gofmt` check, `go vet`, `go mod tidy` check, unit tests with the race detector, golangci-lint, then a multi-arch image build pushed to GHCR |
| `.github/workflows/deploy.yml` | tag `v*`, manual | Pulls the published image on the target host, updates the compose project, restarts, verifies the container, and rolls back on failure |

The `Verify` job runs exactly the commands listed in `AGENTS.md`:

```bash
go fmt ./...
go test ./...
go vet ./...
```

### Image tags

| Tag | Example | Meaning |
|---|---|---|
| `latest` | `ghcr.io/sigdrakon/lite-upf:latest` | latest commit on the default branch |
| branch | `ghcr.io/sigdrakon/lite-upf:main` | latest commit on `main` |
| semver | `ghcr.io/sigdrakon/lite-upf:v0.1.0` | exact release tag |
| semver minor | `ghcr.io/sigdrakon/lite-upf:v0.1` | floating minor release |
| commit | `ghcr.io/sigdrakon/lite-upf:abc1234...` | immutable per-commit build |

---

## Repository layout

```
.
├── .github/workflows/     CI and deployment pipelines
├── cmd/lite-upf/          CLI entry point
├── internal/
│   ├── config/            configuration loading and validation
│   ├── gtpu/              GTP-U header parsing, encoding, N3 listener
│   ├── pfcp/              N4 PFCP listener
│   ├── session/           session store (PDR/FAR state)
│   └── version/           build metadata
├── configs/config.yaml    example runtime configuration
├── scripts/               deploy.sh / rollback.sh (run on the target host)
├── Dockerfile             multi-stage build -> runtime image
└── docker-compose.yml     single-node deployment
```

---

## Setup

### 1. Repository settings

* **Settings → Actions → General → Workflow permissions → Read and write
  permissions** — needed to push the image to GHCR.
* **Settings → Branches → Add rule** on `main` — require status checks
  `Verify`, `Lint` and `Build & publish image` to pass before merging. This is
  what stops a broken commit from reaching `main`; the
  `Commit Message Check` workflow is a separate rule and only runs on pull
  requests, so it stays dormant if you push straight to `main`.

### 2. Deployment secrets

Only required for `deploy.yml`. Set them under
**Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Example | Purpose |
|---|---|---|
| `DEPLOY_HOST` | `10.0.0.21` | target host address |
| `DEPLOY_USER` | `deploy` | SSH user on the target host |
| `DEPLOY_PATH` | `/opt/lite-upf` | directory holding `docker-compose.yml` |
| `DEPLOY_SSH_KEY` | *(private key contents)* | SSH key used to reach the host |
| `DEPLOY_KNOWN_HOSTS` | *(output of `ssh-keyscan -H HOST`)* | host key pinning |

Optional variable (**Settings → Actions → Variables**):

| Variable | Example | Purpose |
|---|---|---|
| `HEALTHCHECK_URL` | `http://10.0.0.21:8080/healthz` | post-deploy smoke test |

> **Note:** `lite-upf` does not expose an HTTP listener today, so
> `HEALTHCHECK_URL` is a no-op until a health endpoint is added. The deploy
> step is skipped with a warning in the meantime; the container-state check in
> `scripts/deploy.sh` is what actually gates the rollout.

### 3. Prepare the target host

```bash
sudo sysctl -w net.ipv4.ip_forward=1
sudo mkdir -p /opt/lite-upf/configs /opt/lite-upf/log
cp docker-compose.yml /opt/lite-upf/
cp configs/config.yaml /opt/lite-upf/configs/
```

---

## Running

### Container

```bash
docker run --rm -it \
  --cap-add=NET_ADMIN --cap-add=NET_RAW \
  -p 8805:8805/udp -p 2152:2152/udp \
  -v "$PWD/configs:/lite-upf/config:ro" \
  ghcr.io/sigdrakon/lite-upf:latest
```

`NET_ADMIN` is required because the forwarding path programs routes and
installs iptables NAT/FORWARD rules; `NET_RAW` is required for raw netlink
sockets.

### Docker Compose

```bash
docker compose up -d
docker compose logs -f lite-upf
```

### Local binary

```bash
make build
./bin/lite-upf -config configs/config.yaml
```

---

## Local development

```bash
make build        # compile ./bin/lite-upf with version metadata
make test         # unit tests
make test-race    # unit tests with the race detector
make cover        # coverage summary
make vet          # go vet
make fmt-check    # fail if the tree is not gofmt clean
make verify       # fmt-check + vet + test + lint
make docker-build # build the container image locally
```

---

## Configuration

All lab-specific values live in the configuration file. The binary contains no
hard-coded addresses, interface names, or paths.

```yaml
n4:
  listen: "0.0.0.0:8805"   # PFCP control plane
n3:
  listen: "0.0.0.0:2152"   # GTP-U access side
n6:
  interface: "eth0"        # data-network side
logging:
  level: "info"            # trace | debug | info | warn | error
features:
  pfcp: true
  gtpu: true
```

The configuration is validated at startup. A missing or malformed value makes
the process exit immediately with an actionable message instead of failing
later at runtime.

---

## Release process

```bash
git checkout main && git pull
git tag -a v0.1.0 -m "release v0.1.0"
git push origin v0.1.0
```

This triggers:

1. `ci.yml` — verify, lint, build and push `v0.1.0`, `v0.1` and the commit SHA
   to GHCR, with SBOM and provenance attestations.
2. `deploy.yml` — deploy `v0.1.0` to the configured host, verify the container
   is running, and roll back automatically if the deploy or the smoke test fails.
