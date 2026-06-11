#!/bin/sh
# sql-db setup: install SQLAlchemy + Postgres/MySQL drivers. SQLite is stdlib.
# Idempotent. Run via code_exec or shell.
set -e

echo "installing SQLAlchemy + psycopg2-binary + PyMySQL ..."
python -m pip install --quiet SQLAlchemy psycopg2-binary PyMySQL 2>/dev/null \
  || pip install --quiet SQLAlchemy psycopg2-binary PyMySQL 2>/dev/null \
  || pip3 install SQLAlchemy psycopg2-binary PyMySQL

echo "sql-db ready: run scripts/db.py '<json-spec>'"
