# Cove

Cove is a Wails v3 desktop and mobile application using Go, React, TypeScript, and pnpm.

## Prerequisites

- Go 1.25+
- pnpm 10+
- Xcode (for iOS development)

## Getting Started

Install dependencies:

```sh
pnpm --dir frontend install
go mod tidy
```

When working from the parent Cove workspace, use the root Makefile as the shared command entry point:

```sh
make app-dev
make app-build
make app-frontend-test
make app-mobile-test
```

These targets delegate to this repository's existing `Taskfile.yml` and package scripts.

## Development

### Desktop

Run the desktop app with hot reload:

```sh
wails3 dev -config ./build/config.yml
# or
task dev
```

### iOS Simulator

Run on iOS Simulator with Vite hot reload:

```sh
task ios:dev
```

This task automatically:
1. Boots an iOS Simulator
2. Starts the Vite dev server (if not already running)
3. Compiles the Go backend as a C archive with `devServerURL` injected
4. Builds the `.app` bundle, signs it, installs and launches on the simulator

Open the generated Xcode project for manual debugging:

```sh
task ios:xcode
```

Stream logs from the simulator:

```sh
task ios:logs
task ios:logs:dev
```

## Frontend

The frontend uses React 19, Vite 8, and TypeScript 7.

```sh
cd frontend

pnpm dev          # Start Vite dev server
pnpm build        # Production build (tsc + vite build)
pnpm build:dev    # Development build without minification
pnpm test         # Run unit tests with vitest
pnpm preview      # Preview production build
```

### Authentication

The app includes a built-in auth flow (`frontend/src/features/auth`):
- Login and registration screens
- Session persistence and restore
- Token-based API communication with field-level error handling

## Production Build

Build for the current platform:

```sh
task build
```

Build for a specific platform:

```sh
task darwin:build
task windows:build
task linux:build
task ios:build
task android:build
```

Package for distribution:

```sh
task package
```

### Server Mode (No GUI)

Build and run as a headless HTTP server:

```sh
task build:server
task run:server
```

### Docker

Build and run via Docker for server mode deployment:

```sh
task build:docker
task run:docker
task setup:docker   # Build cross-compilation image
```

## Testing

The frontend uses Vitest with Testing Library:

```sh
cd frontend && pnpm test
```

## Project Structure

```
├── main.go                  # Entry point, embeds frontend assets, starts the app
├── internal/
│   ├── app/                 # Wails app configuration, services, main window
│   ├── services/            # Wails-bound backend services
│   ├── domain/              # Wails-independent business types
│   └── platform/            # System-specific helpers
├── frontend/
│   ├── src/
│   │   ├── app/             # React application shell (App component, routing)
│   │   ├── features/        # Feature modules (auth, ...)
│   │   ├── shared/          # Reusable utilities and API wrappers
│   │   └── styles/          # Global styles
│   └── bindings/            # Auto-generated Wails bindings
├── build/
│   ├── config.yml           # Wails build configuration
│   ├── appicon.png          # App icon source
│   ├── darwin/              # macOS build tasks and assets
│   ├── windows/             # Windows build tasks and assets
│   ├── linux/               # Linux build tasks and assets
│   ├── ios/                 # iOS build pipeline
│   │   ├── scripts/         # Build helpers (deps, overlay patching)
│   │   └── xcode/           # Generated Xcode project & full-bleed overlay
│   └── android/             # Android build tasks and assets
├── Taskfile.yml             # Top-level task definitions
└── logo/                    # Logo assets (standard and full-bleed)
```

## License

MIT
