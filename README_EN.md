# gitgit

A tool for managing GitLab group project structures locally.

[Русская версия](README.md)

## Description

gitgit recursively clones and updates all projects in a given GitLab group. It uses goroutines to work with multiple repositories in parallel.

Features:
- Recursive cloning of all group projects (including subgroups)
- Updating (`git pull --all`) already cloned projects
- Project filtering by regular expression
- Parallel execution with configurable number of workers
- Cloning via SSH (default) or HTTPS

## Installation

### Build from source

```bash
go build -o gitgit ./cmd/main.go
```

### Install from releases

Download a pre-built binary from the [Releases](https://github.com/laduwka/gitgit/releases) page. Builds are available for Linux, macOS, and Windows (amd64/arm64).

### Docker

```bash
docker build . -t gitgit
```

## Usage

### Token setup

The token can be passed via the `-token` flag or an environment variable:

```bash
export TOKEN="your_gitlab_token"
```

Create the token in GitLab profile settings: **Settings → Access Tokens**.

### Examples

Clone all projects in a group:

```bash
./gitgit -id 567
```

Update already cloned projects (re-run):

```bash
./gitgit -id 567
```

Filter by project path:

```bash
./gitgit -id 567 -regex 'backend/services'
```

Increase the number of parallel workers:

```bash
./gitgit -id 567 -workers 8
```

Use HTTPS instead of SSH:

```bash
./gitgit -id 567 -http
```

Specify a custom GitLab instance:

```bash
./gitgit -id 567 -url https://git.example.com/api/v4
```

The URL can also be set via an environment variable:

```bash
export URL="https://git.example.com/api/v4"
./gitgit -id 567
```

### All flags

| Flag | Default | Description |
|------|---------|-------------|
| `-id` | — | GitLab group ID (required) |
| `-url` | `https://gitlab.com/api/v4` | GitLab API URL |
| `-token` | `$TOKEN` | GitLab private token |
| `-data` | `./<group_id>` | Working directory |
| `-regex` | `.` | Regular expression to filter projects |
| `-workers` | `4` | Number of parallel workers |
| `-http` | `false` | Clone via HTTPS instead of SSH |

## Docker

```bash
docker build . -t gitgit

docker run -v "$PWD:/data:rw" \
  -v "$HOME/.ssh:/.ssh" \
  -v "$SSH_AUTH_SOCK:/.SSH_AUTH_SOCK" \
  -e TOKEN \
  -u "$UID:$UID" \
  --rm gitgit -id 567
```

Convenience alias:

```bash
alias gitgit='docker run --rm -v "$PWD:/data:rw" \
  -v "$HOME/.ssh:/.ssh" \
  -v "$SSH_AUTH_SOCK:/.SSH_AUTH_SOCK" \
  -e TOKEN -u "$UID:$UID" gitgit'
```

macOS alias example:

```bash
alias gitgit='docker run --rm -it \
  -v /run/host-services/ssh-auth.sock:/ssh-agent \
  -e SSH_AUTH_SOCK="/ssh-agent" \
  -v "$HOME/.ssh/known_hosts:/root/.ssh/known_hosts" \
  -v "$PWD:/data:rw" \
  -e TOKEN gitgit'
```

## How it works

1. Fetches the project list via GitLab API v4 (`/groups/:id/projects`) with pagination
2. Filters out archived projects and applies the regular expression
3. Processes projects in parallel: clones new ones or updates existing ones
4. Projects are stored as `<working_directory>/<path_with_namespace>`, mirroring GitLab's hierarchy

## Development

```bash
go test ./...                              # run tests
go build -o gitgit ./cmd/main.go               # build
```

Releases are created automatically via [GoReleaser](https://goreleaser.com/) when a `v*` tag is pushed.

## Links

- [GitLab API — Groups](https://docs.gitlab.com/ee/api/groups.html)
- [GitLab API — Projects](https://docs.gitlab.com/ee/api/projects.html)
