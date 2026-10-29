# Sidecar Protocol (v1)

The sibling repositories ship helper binaries (`dendrite-core`, `dendrite-git`, `dendrite-rag`) that Dendrite calls through `pkg/sidecar`. **Rule: when a Rust service exists, Dendrite only calls it; the Go copy is deleted.** `dendrite-plane` and `dendrite-rag` are required: when one is missing or speaks another protocol major, the call returns an error that names the fix (`dendrite install`), never a silent Go fallback (`planesvc.ErrSidecarMissing`). `dendrite soma diagnose` reports where each was found. Sidecars not yet built (`dendrite-core`, `dendrite-git`) still have Go implementations and may use `sidecar.Call` with a fallback until their Rust service ships.

## Transport

- The sidecar is spawned as a child process with no arguments.
- It speaks JSON-RPC 2.0 over stdin/stdout, **one JSON object per line** (newline-delimited, no `Content-Length` framing).
- Requests are serialised: the client sends one request and reads exactly one response line before sending the next.
- stderr is passed through to Dendrite's stderr for diagnostics.
- When stdin closes, the sidecar must exit. Dendrite kills it if it is still running 5 s later.

## Handshake

The first request is always `health`. Its result must be:

```json
{"name": "dendrite-core", "version": "0.3.1", "protocol": "1.2.0"}
```

`protocol` is the semver version of this contract. A client built for protocol major `N` refuses a sidecar with a different major (and uses its fallback only where a Go implementation still exists). Minor versions add methods; patch versions never change shapes.

## Errors

Failures use the standard JSON-RPC `error` object: `{"code": -32601, "message": "method not found: x"}`. Where a Go fallback still exists, any error makes `sidecar.Call` use it and report `origin: fallback` with the reason. Required sidecars (`dendrite-plane`, `dendrite-rag`) return the error to the caller instead; the sidecar's `kind` (`unauthorized`, `not_found`) is mapped back to the sentinel errors callers test with `errors.Is`.

## Locating binaries

`sidecar.DefaultLocator().Locate(name)` returns the first executable it finds, checking in this order:

1. `$DENDRITE_<NAME>_BIN` (for example `DENDRITE_DENDRITE_CORE_BIN`). If this is set but does not point to an executable, the lookup fails with an error instead of continuing.
2. `<member>/target/release/<name>`, then `<member>/bin/<name>`, for every local member of every synapse. This is develop mode: a `cargo build --release` in `../dendrite-core-rs` is used immediately. Members switched to `source = "remote"` are skipped.
3. `$DENDRITE_HOME/bin/<name>`, which is where `dendrite update` installs builds.
4. `PATH`.
5. `~/.cargo/bin/<name>`, where `cargo install` puts binaries.

## Method catalogue

Methods are versioned in each sidecar repository's `proto/*.json` (DEN-972..974). The Go side reaches each method through a `sidecar.Spec{Binary, Major, Method}` together with its fallback.

### Graph methods (`dendrite-rag` 1.1.0, DRAG-32)

`dendrite-rag` serves the typed code graph and Leiden community analysis:
- `graph.communities`: `{root, limit?, min_size?}` -> `{communities}`
- `graph.god_nodes`: `{root, min_in_degree?, limit?}` -> `{nodes}`
- `graph.path`: `{root, from, to, kinds?, max_hops?}` -> `{found, directed, level, steps}`
- `graph.explain`: `{root, target}` -> `{matches}`

Every graph method returns error kind `not_indexed` before the first index; Dendrite calls `index` first. Surfaced in Dendrite via `pkg/rag`, MCP tools (`dendrite_graph_*`), and HTTP (`/api/v1/graph/*`). DEND-201/202 are delivered natively by `dendrite-rag graph.*` and these tools.
