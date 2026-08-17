#!/bin/bash
set -e

echo "Building dependencies..."
ROOT_DIR=$(pwd)
export CGO_CFLAGS="-I$ROOT_DIR/include"
export CGO_LDFLAGS="$ROOT_DIR/lib/linux_amd64/liblancedb_go.a -lm -ldl -lpthread"

cmake -B build .
cmake --build build --parallel 8

echo "Starting loom server..."
./loom serve &
LOOM_PID=$!

echo "Waiting for loom server to be ready..."
for i in {1..30}; do
    if curl -s http://127.0.0.1:11434/ > /dev/null; then
        echo "Server is up!"
        break
    fi
    sleep 2
done

echo "Running import script..."
./import_models.sh

echo "Shutting down loom server..."
kill $LOOM_PID
