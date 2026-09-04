#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

migration_files=$(find migrations -maxdepth 1 -type f \
    \( -name '*.up.sql' -o -name '*.down.sql' \) -print | LC_ALL=C sort)

if [ -z "$migration_files" ]; then
    echo 'No migrations found.' >&2
    exit 1
fi

for up_file in migrations/*.up.sql; do
    down_file=${up_file%.up.sql}.down.sql
    if [ ! -f "$down_file" ]; then
        echo "Missing down migration for $up_file" >&2
        exit 1
    fi
done

for down_file in migrations/*.down.sql; do
    up_file=${down_file%.down.sql}.up.sql
    if [ ! -f "$up_file" ]; then
        echo "Missing up migration for $down_file" >&2
        exit 1
    fi
done

expected_sequence=1
for up_file in migrations/*.up.sql; do
    filename=${up_file##*/}
    version=${filename%%_*}
    expected_version=$(printf '%06d' "$expected_sequence")
    if [ "$version" != "$expected_version" ]; then
        echo "Expected migration version $expected_version, found $filename" >&2
        exit 1
    fi
    expected_sequence=$((expected_sequence + 1))
done

sha256sum --check migrations/checksums.sha256
