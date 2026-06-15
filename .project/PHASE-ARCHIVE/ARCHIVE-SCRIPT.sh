#!/bin/bash
# PHASE Archive Script
# Run from: D:/Codebox/PROJECTS/AGEZT/.project
# Creates: PHASE-ARCHIVE/by-year/ subdirectories

set -e

ARCHIVE_DIR="PHASE-ARCHIVE"
SOURCE_DIR="."

echo "=== PHASE Archive Migration ==="
echo "Source: $SOURCE_DIR/PHASE-*.md"
echo "Destination: $ARCHIVE_DIR/"
echo ""

# Create year-based subdirectories
mkdir -p "$ARCHIVE_DIR/by-year/2024"
mkdir -p "$ARCHIVE_DIR/by-year/2025"
mkdir -p "$ARCHIVE_DIR/by-year/2026"

# Count files
TOTAL=$(ls -1 PHASE-*.md 2>/dev/null | wc -l)
echo "Total PHASE files found: $TOTAL"
echo ""

# Recent reports (M900+) - keep in root archive, don't move
echo "Recent reports (M900-M929) - keeping reference in SUMMARY"
# These stay accessible but are noted in the index

# M800-M899 -> 2025
echo "Moving M800-M899 files..."
for f in PHASE-M8*.md; do
  [ -f "$f" ] && mv "$f" "$ARCHIVE_DIR/by-year/2025/" 2>/dev/null || true
done

# M600-M799 -> 2025
echo "Moving M600-M799 files..."
for f in PHASE-M[6-7][0-9][0-9].md; do
  [ -f "$f" ] && mv "$f" "$ARCHIVE_DIR/by-year/2025/" 2>/dev/null || true
done

# M400-M599 -> 2025
echo "Moving M400-M599 files..."
for f in PHASE-M[4-5][0-9][0-9].md; do
  [ -f "$f" ] && mv "$f" "$ARCHIVE_DIR/by-year/2025/" 2>/dev/null || true
done

# M200-M399 -> 2024
echo "Moving M200-M399 files..."
for f in PHASE-M[2-3][0-9][0-9].md; do
  [ -f "$f" ] && mv "$f" "$ARCHIVE_DIR/by-year/2024/" 2>/dev/null || true
done

# M0-M199 -> 2024
echo "Moving M0-M199 files..."
for f in PHASE-M[01][0-9]*.md PHASE-M[0-9].md PHASE-M[0-9][0-9].md; do
  [ -f "$f" ] && mv "$f" "$ARCHIVE_DIR/by-year/2024/" 2>/dev/null || true
done

# Verify counts
echo ""
echo "=== Archive Complete ==="
echo "2024: $(ls -1 $ARCHIVE_DIR/by-year/2024/*.md 2>/dev/null | wc -l) files"
echo "2025: $(ls -1 $ARCHIVE_DIR/by-year/2025/*.md 2>/dev/null | wc -l) files"
echo "2026: $(ls -1 $ARCHIVE_DIR/by-year/2026/*.md 2>/dev/null | wc -l) files"
echo "Remaining: $(ls -1 PHASE-*.md 2>/dev/null | wc -l) files"
