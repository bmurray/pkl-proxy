# pkl-proxy

Private [Pkl](https://pkl-lang.org) packages on GitHub, without the headache.

Once set up, resolving private packages is a single command:

```bash
pkl-proxy pkl project resolve
```

That's it. Authentication, token refresh, and request routing are all handled transparently.

## Why

Pkl has no built-in support for private GitHub repositories. If your team hosts Pkl packages in private repos, there's no native way to authenticate when resolving `package://pkg.pkl-lang.org/github.com/...` dependencies.

pkl-proxy fixes this. It runs a local HTTP proxy that authenticates as a GitHub App and rewrites Pkl's requests to route through it. Tokens auto-refresh, and Pkl's built-in caching means you only need the proxy running for the initial resolve.

## Installation

### Pre-built Binaries

Download the latest binary for your platform from the [GitHub Releases](https://github.com/bmurray/pkl-proxy/releases) page.

Available platforms: `darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`, `windows-amd64`.

```bash
# Example: macOS Apple Silicon
curl -Lo pkl-proxy https://github.com/bmurray/pkl-proxy/releases/latest/download/pkl-proxy-darwin-arm64
chmod +x pkl-proxy
sudo mv pkl-proxy /usr/local/bin/
```

### From Source

```bash
go install github.com/bmurray/pkl-proxy@latest
```

## Configuration

### 1. Create a GitHub App

You need a GitHub App with read access to your private repositories. You can create one for your personal account or for an organization.

#### For a Personal Account

1. Go to **Settings > Developer settings > GitHub Apps > New GitHub App**
   - Or navigate directly to https://github.com/settings/apps/new
2. Fill in the required fields:
   - **GitHub App name**: Something like `pkl-proxy` or `my-pkl-downloader`
   - **Homepage URL**: Can be anything (e.g. `https://github.com`)
   - **Webhook**: Uncheck "Active" (not needed)
3. Under **Permissions > Repository permissions**:
   - **Contents**: Read-only
4. Under **Where can this GitHub App be installed?**:
   - Select "Only on this account"
5. Click **Create GitHub App**

#### For an Organization

1. Go to your org's **Settings > Developer settings > GitHub Apps > New GitHub App**
   - Or navigate to `https://github.com/organizations/<YOUR_ORG>/settings/apps/new`
2. Fill in the same fields as above
3. Under **Permissions > Repository permissions**:
   - **Contents**: Read-only
4. Under **Where can this GitHub App be installed?**:
   - Select "Only on this account" (recommended) or "Any account"
5. Click **Create GitHub App**

### 2. Get the Client ID or App ID

After creating the app, you'll be on the app's settings page.

- The **Client ID** (preferred) is a string displayed near the top (e.g. `Iv23liABCDEFGH12345`)
- The **App ID** is a numeric ID displayed above it (e.g. `123456`)

You only need one of these — see [Authentication Modes](#authentication-modes) below.

### 3. Generate a Private Key

1. On the app's settings page, scroll down to **Private keys**
2. Click **Generate a private key**
3. A `.pem` file will be downloaded automatically
4. Move this file to your pkl-proxy config directory (see below)

### 4. Install the App on Your Account or Organization

1. On the app's settings page, click **Install App** in the left sidebar
2. Choose the account (personal or org) to install on
3. Select **All repositories** (preferred) or choose specific repos
4. Click **Install**

Installing on the entire account is recommended so you don't have to reconfigure every time you add a new private Pkl package repo.

If the app is installed on multiple accounts, you'll need the **Installation ID** to tell pkl-proxy which one to use. After installation, look at the URL in your browser:
```
https://github.com/settings/installations/12345678
```
The number at the end (`12345678`) is your Installation ID. If you only have one installation, pkl-proxy will auto-discover it.

### 5. Create the Config File

Create a config directory in one of these locations (checked in order):

| Platform | Primary Location | Fallback |
|----------|-----------------|----------|
| macOS | `~/Library/Application Support/pkl-proxy/` | `~/.pkl-proxy/` |
| Linux | `~/.config/pkl-proxy/` | `~/.pkl-proxy/` |
| Windows | `%AppData%/pkl-proxy/` | `~/.pkl-proxy/` |

Place your private key `.pem` file in this directory, then create a config file.

#### Using Pkl (recommended)

Create `config.pkl`:

**Client ID** (recommended):

```pkl
privateKey = "your-app-name.2025-01-01.private-key.pem"
clientId = "Iv23liABCDEFGH12345"
```

**App ID**:

```pkl
privateKey = "your-app-name.2025-01-01.private-key.pem"
appId = 123456
```

Both modes auto-discover the installation. If the app is installed on multiple accounts, add `installationId` to select one:

```pkl
installationId = 12345678
```

#### Using JSON

```json
{
    "privateKey": "your-app-name.2025-01-01.private-key.pem",
    "clientId": "Iv23liABCDEFGH12345"
}
```

#### Configuration Options

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `privateKey` | String | Yes | - | Path to the GitHub App private key `.pem` file. Relative paths resolve against the config directory. |
| `clientId` | String | No* | - | GitHub App Client ID (recommended) |
| `appId` | Int | No* | - | GitHub App ID (numeric) |
| `installationId` | Int | No | - | GitHub App Installation ID. Auto-discovered if omitted. Required when the app has multiple installations. |
| `listenAddress` | String | No | `localhost:9443` | Address for the local proxy server |

\* Either `clientId` or `appId` must be set. If both are set, `appId` takes precedence.

Config files are loaded in priority order: `config.pklbin` > `config.pkl` > `config.json`.

## Usage

### Register Private Repos

Tell pkl-proxy which GitHub users/orgs have private Pkl packages:

```bash
# Proxy all repos under a GitHub user or org
pkl-proxy install github.com/myorg

# Proxy a specific repo only
pkl-proxy install github.com/myorg/my-private-package

# Also accepts pkg.pkl-lang.org paths
pkl-proxy install pkg.pkl-lang.org/github.com/myorg
```

This writes rewrite rules to `~/.pkl/pkl-proxy/rewrites.pkl`.

To remove a path:

```bash
pkl-proxy uninstall github.com/myorg
```

### Wire Into Pkl Settings

After installing paths, connect the rewrites to Pkl's settings:

```bash
pkl-proxy settings install
```

This modifies `~/.pkl/settings.pkl` to import the proxy rewrites. If the file doesn't exist, it creates one. If there are conflicting manual rewrite entries, it will warn you and ask you to remove them first.

To disconnect:

```bash
pkl-proxy settings uninstall
```

### Run a Command Through the Proxy

Wrap any command with `pkl-proxy run` to start the proxy for the duration of that command:

```bash
# Resolve Pkl project dependencies through the proxy
pkl-proxy run pkl project resolve

# Evaluate a Pkl file that imports private packages
pkl-proxy run pkl eval myconfig.pkl

# The "run" keyword is optional
pkl-proxy pkl project resolve
```

The subprocess receives the `PKL_PROXY_LISTEN_ADDRESS` environment variable so Pkl can resolve the correct proxy address at evaluation time.

> **Tip:** Pkl caches resolved packages locally. Once you've successfully run `pkl project resolve` through the proxy, subsequent `pkl eval` commands will use the cached packages and won't need the proxy running.

### Daemon Mode

Run the proxy as a long-lived server, useful for CI/CD pipelines or Docker containers:

```bash
pkl-proxy daemon
```

The daemon:
- Responds to `SIGINT` and `SIGTERM` with graceful shutdown
- Reaps orphaned child processes when running as PID 1 (Docker)

#### Docker Example

```dockerfile
FROM golang:1.23 AS builder
RUN go install github.com/bmurray/pkl-proxy@latest

FROM alpine:latest
COPY --from=builder /go/bin/pkl-proxy /usr/local/bin/pkl-proxy
COPY config.pkl /etc/pkl-proxy/config.pkl
COPY private-key.pem /etc/pkl-proxy/private-key.pem

# Use absolute path for private key when config dir differs from key location
# Or place both files in the same directory

CMD ["pkl-proxy", "daemon"]
```

## How the Rewrite System Works

When you run `pkl-proxy install github.com/myorg`, the tool generates `~/.pkl/pkl-proxy/rewrites.pkl`:

```pkl
local listenAddress = read?("env:PKL_PROXY_LISTEN_ADDRESS") ?? "localhost:9443"

local paths: Listing<String> = new {
  "myorg"
}

rewrites: Mapping<String, String> = new {
  for (path in paths) {
    ["https://github.com/\(path)/"] = "http://\(listenAddress)/\(path)/"
    ["https://pkg.pkl-lang.org/github.com/\(path)/"] = "http://\(listenAddress)/\(path)/"
  }
}
```

Then `pkl-proxy settings install` adds this to `~/.pkl/settings.pkl`:

```pkl
amends "pkl:settings"

import "pkl-proxy/rewrites.pkl" as pklProxy

http {
  rewrites {
    for (key, value in pklProxy.rewrites) {
      [key] = value
    }
  }
}
```

This tells Pkl to route requests for your private repos through the local proxy, which handles authentication transparently.

## Commands

| Command | Description |
|---------|-------------|
| `pkl-proxy install <path>` | Add a GitHub path to proxy rewrites |
| `pkl-proxy uninstall <path>` | Remove a GitHub path from proxy rewrites |
| `pkl-proxy settings install` | Wire rewrites into `~/.pkl/settings.pkl` |
| `pkl-proxy settings uninstall` | Remove rewrites from `~/.pkl/settings.pkl` |
| `pkl-proxy daemon` | Start proxy as a long-lived server |
| `pkl-proxy run <cmd> [args]` | Start proxy and run a command |
| `pkl-proxy <cmd> [args]` | Shorthand for `pkl-proxy run <cmd> [args]` |

## License

Apache 2.0
