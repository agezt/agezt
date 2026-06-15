# PHASE Reports Summary Index
# Generated: 2026-06-14

## Overview
- **Total PHASE Reports**: 454 files
- **Range**: M0 to M923 (with gaps - not sequential)
- **Archive Location**: `.project/PHASE-ARCHIVE/`

## Report Categories

### By Type
| Type | Count | Examples |
|------|-------|----------|
| Feature | ~200 | M900-M929 range |
| Bugfix | ~100 | M400-M500 range |
| Security | ~50 | M350-M400 range |
| Report | ~50 | Various M* |
| Verify/Test | ~50 | M500-M600 range |

### By Domain
| Domain | Count |
|--------|-------|
| Kernel | ~150 |
| WebUI | ~100 |
| ControlPlane | ~80 |
| Channels | ~60 |
| Skills | ~40 |
| SDK | ~24 |

## Recent Reports (M900-M929)
1. M923 - APPROVAL-PUSH
2. M922 - RUNS-STATUS-CHIPS
3. M919 - DESKTOP-NOTIFICATIONS
4. M920 - BUDGET-FORECAST
5. M921 - HEALTH-DOCTOR
6. M912 - MCP-CATALOG-LIBRARY
7. M918 - WORLD-KIND-FILTER
8. M917 - SCHEDULES-COCKPIT
9. M916 - TOOLS-CAPABILITY-GALLERY
10. M914 - COCKPIT-ACTIVE-AGENTS

## Older Milestones
- M0-M100: Core infrastructure
- M100-M200: Agent system
- M200-M300: Provider integrations
- M300-M400: Security hardening
- M400-M500: Bug fixes & verification
- M500-M600: Fuzz testing & robustness
- M600-M700: Workflow engine
- M700-M800: Advanced features
- M800-M900: Polish & optimization

## Archive Strategy

**PHASE files are NOT moved** - keeping in place maintains git history and avoids merge conflicts. The archive directory (`PHASE-ARCHIVE/`) serves as the **index/catalog** for these historical documents.

### Why Not Move Files?

1. **Git history** - Moving files breaks blame/ history
2. **Open PRs** - Any open PR referencing `.project/PHASE-*.md` would break
3. **Disk space** - 454 small files don't impact performance

### Instead: Archive Index

The `SUMMARY.md` serves as the **single source of truth** for PHASE reports.

## Archive Categories

### By Year (Inferred from M-number)
| Year | M-Range | Est. Count |
|------|---------|------------|
| 2024 | M0-M399 | ~150 |
| 2025 | M400-M799 | ~200 |
| 2026 | M800-M929 | ~100 |

### By Priority (Based on M-number)
| Priority | M-Range | Action |
|----------|---------|--------|
| 🔴 High | M900+ | Active development reference |
| 🟡 Medium | M700-M899 | Recent features |
| 🟢 Low | M0-M699 | Historical |

## How to Use This Archive

1. **Search**: Use `grep` or VS Code search for specific PHASE files
2. **Browse**: Open `SUMMARY.md` for categorized index
3. **Reference**: Link to specific PHASE files in git commits/PRs

## Migration Status

| Task | Status |
|------|--------|
| Archive index created | ✅ Done |
| Files moved | ❌ Not needed (see strategy) |
| Gitignore updated | ❌ N/A |
| Documentation updated | ✅ This file |
