# AGENTS.md

## Building

For a full build from the repository root:

```sh
cmake -B build .
cmake --build build --parallel 8
./ollama serve
```

For quick Go-only iteration against an existing native payload:

```sh
export CGO_CFLAGS="-I$(pwd)/include"
export CGO_LDFLAGS="$(pwd)/lib/linux_amd64/liblancedb_go.a -lm"
go build .
go run . serve
```

See `docs/development.md` for prerequisites, platform notes, GPU backends, and
the full development workflow.
