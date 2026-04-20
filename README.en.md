# SubForge

[简体中文](README.md) | [English](README.en.md)

**SubForge** is a lightweight Clash subscription manager and distribution platform written in Go, with a built-in Web UI ready to use out of the box.

It aggregates nodes from multiple subscription sources and, with custom service rules, Hosts and DNS presets, generates clean and fully controllable Clash configuration files for different devices.

## Features

- **Subscription management** — Add multiple subscriptions, fetch nodes manually or on a schedule, with traffic statistics and expiry display
- **Node management** — Custom aliases, enable/disable, alias preset rules, manually add local nodes
- **Service rules** — 22 built-in routing rule sets (Google, YouTube, OpenAI, Steam, etc.); add remote/local rules, reorder and toggle them; "Direct rule" switch skips the dedicated proxy-group
- **Hosts / DNS presets** — Built-in DNS preset; add custom DNS / Hosts configurations and bind them to different distributions
- **Distribution profiles** — Create independent distribution links for each device:
  - Pick nodes and service rules per profile
  - Per-profile GEOIP CN direct, catch-all, DNS / Hosts preset
  - Override the default proxy of each service rule per profile
  - One-click "Sync Rules" to incrementally sync from the global rule set while preserving existing toggles and node settings
  - Live preview of the generated YAML
- **Multilingual UI** — Built-in 中文 / English switcher; mobile-friendly responsive layout
- **Web UI** — Single-file Vue 3 frontend, no separate build step required
- **Auth & security** — Cookie session auth, bcrypt password hashing; optional browser-only local mode (data stored in localStorage, no backend account needed)

## Screenshots

<!-- TODO: replace with real screenshots -->

| Subscriptions | Nodes |
|:---:|:---:|
| ![Subscriptions](docs/screenshots/subscriptions.png) | ![Nodes](docs/screenshots/nodes.png) |

| Service Rules | Distribution |
|:---:|:---:|
| ![Rules](docs/screenshots/services.png) | ![Profiles](docs/screenshots/profiles.png) |

## Quick Start

### Docker Compose (recommended)

Create a `docker-compose.yaml` with the following content:

```yaml
services:
  subforge:
    image: teacat99/subforge:latest
    container_name: subforge
    restart: unless-stopped
    ports:
      - "8080:8080"    # left side can be changed to a custom port
    volumes:
      - subforge-data:/data
    environment:
      - TZ=Asia/Shanghai
      - SUBFORGE_LOGIN_ENABLED=true   # default: true; set to false to hide the login page
      - SUBFORGE_LOCAL_ENABLED=false  # default: false; set to true to enable browser local mode

volumes:
  subforge-data:
```

Then start:

```bash
docker compose up -d
```

The service runs on `http://localhost:8080` by default.

Default admin credentials: `admin` / `passwd` (please change immediately after the first login).

> The GHCR mirror is also available — replace `teacat99/subforge:latest` with `ghcr.io/teacat99/subforge:latest`.

### Docker CLI

```bash
docker run -d \
  --name subforge \
  --restart unless-stopped \
  -p 8080:8080 \
  -v subforge-data:/data \
  -e TZ=Asia/Shanghai \
  -e SUBFORGE_LOGIN_ENABLED=true \
  -e SUBFORGE_LOCAL_ENABLED=false \
  teacat99/subforge:latest
```

### Build from source

Requires Go 1.24+

```bash
git clone https://github.com/teacat99/SubForge.git
cd SubForge

# build
go build -o subforge ./cmd/server/

# run with default login mode
./subforge -port 8080

# disable login mode and enable browser local mode
SUBFORGE_LOGIN_ENABLED=false SUBFORGE_LOCAL_ENABLED=true ./subforge -port 8080
```

### CLI flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | Listen port |
| `-db` | `data/subforge.db` | SQLite database path (relative to the executable directory) |

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TZ` | `UTC` | Container timezone, recommend `Asia/Shanghai` (or your local TZ) |
| `SUBFORGE_LOGIN_ENABLED` | `true` | Enable login mode (account + server-side storage). Set to `false` to hide the login page |
| `SUBFORGE_LOCAL_ENABLED` | `false` | Enable Web local mode (data stored in browser localStorage) |

> When both `SUBFORGE_LOGIN_ENABLED` and `SUBFORGE_LOCAL_ENABLED` are `false`, the server falls back to login mode automatically to keep the service usable.

## Resource usage (reference, varies with subscription scale)

- **Image size**: ~15 MB (based on `gcr.io/distroless/static`)
- **Memory**: ~20 MB idle; ~40 MB with a typical workload (5 subscriptions + a few hundred nodes)
- **CPU**: < 1% idle; brief single-core spike during scheduled fetching
- **Disk**: SQLite database starts at ~200 KB and grows roughly linearly with the number of nodes / rules
- **Minimum recommended**: 1 vCPU / 128 MB RAM / 200 MB disk

## Project structure

```
SubForge/
├── cmd/server/          # entry point
├── internal/
│   ├── api/             # HTTP API (Gin)
│   ├── generator/       # Clash config generator
│   ├── model/           # data models (GORM)
│   ├── rule/            # rule management & built-in rules
│   ├── store/           # database layer
│   └── subscription/    # subscription parser (YAML/Base64)
├── web/                 # frontend (single-file Vue 3)
├── Dockerfile
├── docker-compose.yaml
└── go.mod
```

## Acknowledgements

SubForge is built on top of these excellent open-source projects and services:

| Project | Used for |
|---------|----------|
| [blackmatrix7/ios_rule_script](https://github.com/blackmatrix7/ios_rule_script) | Routing rules data source |
| [favicon.im](https://favicon.im) | Website favicon service |
| [Gin](https://github.com/gin-gonic/gin) | Go HTTP web framework |
| [GORM](https://gorm.io) | Go ORM framework |
| [glebarez/sqlite](https://github.com/glebarez/sqlite) | Pure-Go SQLite driver |
| [Vue 3](https://vuejs.org) | Frontend framework (loaded from CDN) |

## Contributing

Issues and pull requests are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) first for the contribution guidelines.

## License

This project is licensed under the [Apache License 2.0](LICENSE).

Copyright 2026 teacat99
