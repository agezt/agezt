# Scripts

Project support scripts that do not need to live in the repository root.

- `build.sh` — Go build/test/vet helper. Run from the repository root as `./scripts/build.sh [build|test|race|clean|vet]`.
- `dev.ps1` — Windows development loop that builds `bin\agezt.exe`/`bin\agt.exe`, seeds the isolated `.dev-home`, and runs the daemon. Run from the repository root as `./scripts/dev.ps1`.
- `ci-go-retry.sh`, `e2e-smoke.sh`, `webui-e2e.*` — CI and smoke/e2e helpers.
- `dev/` — ad-hoc developer utilities that are not part of the product runtime.
