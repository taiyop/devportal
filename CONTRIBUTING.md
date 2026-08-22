# Contributing to DevPortal

Thank you for your interest in contributing to DevPortal! We welcome bug reports, feature requests, documentation improvements, and pull requests.

## Development Setup

### Prerequisites

- **Go**: 1.25+ (required for backend)
- **Node.js / Bun**: Node.js 20+ or [Bun](https://bun.sh/) (recommended)
- **Wails v3 CLI**: [Wails v3 beta](https://v3alpha.wails.io/)

Install Wails v3 CLI (if you haven't already):
```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

### Clone & Install

```bash
git clone https://github.com/taiyop/devportal.git
cd devportal

# Install frontend dependencies
bun install
# or: npm install
```

### Running Locally (Development Mode)

```bash
# Start Wails v3 dev mode (hot-reloading frontend + Go backend)
bun run wails:dev
```

To run only the frontend in a browser (with mock bindings):
```bash
bun run dev
```

## Running Tests

Before submitting a Pull Request, please ensure all tests and type checks pass:

```bash
# 1. Run Go backend tests
cd src-wails
go test ./...
cd ..

# 2. Run TypeScript check & Frontend build
bun run build
```

## Pull Request Guidelines

1. **Fork & Branch**: Create a feature branch with a descriptive name (e.g. `feat/auto-proxy-ssl` or `fix/log-streaming`).
2. **Code Style**:
   - Go: Follow standard Go conventions (`go fmt` / `go vet`).
   - TypeScript/React: Follow the existing ESLint and code structure.
3. **Bindings**:
   - If you modify Go structs or services exported to Wails, re-generate the bindings with `wails3` and commit them under `src/bindings/`.
4. **Descriptive PRs**: Explain what changed, why, and how to test your changes.

## Reporting Issues

- Use the GitHub Issue Tracker to report bugs or request features.
- Please include your OS version, Go/Node/Bun versions, and reproducible steps or logs where applicable.

## License

By contributing to DevPortal, you agree that your contributions will be licensed under the [MIT License](LICENSE).
