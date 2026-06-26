# AGEZT Install

This is the short, repeatable setup path for coding agents and new contributors.

## Prerequisites

- Go 1.26.4 or newer
- Node.js 22 or newer
- npm

## Bootstrap

```powershell
go mod download
cd frontend
npm install
cd ..
```

## Verify

```powershell
go test ./...
cd frontend
npm run typecheck
npm test
cd ..
```

## Build

```powershell
go build -o agezt.exe ./cmd/agezt
go build -o agt.exe ./cmd/agt
cd frontend
npm run build
cd ..
```

## Configure A Provider

Use the CLI or environment variables. Do not commit real credentials.

```powershell
.\agt.exe provider creds set openai
.\agt.exe provider reload
```

For local defaults, copy `.env.example` to your own `.env` and edit it locally.

## Useful Focused Checks

```powershell
go test ./kernel/controlplane ./kernel/settings ./contract/fixtures
cd frontend
npm test -- src/lib/chat.test.ts src/views/Chat.context.test.tsx src/views/ConfigCenter.test.tsx
npm run typecheck
cd ..
```
