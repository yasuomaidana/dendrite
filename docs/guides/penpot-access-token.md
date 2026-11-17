# Penpot access token and exporter

Penpot runs from `stacks/compose.penpot.yaml` (`dendrite motor start penpot`). Since DEN-835 the stack has:

| Service | Port | Used for |
|---|---|---|
| `penpot-frontend` | 9000 | the web UI |
| `penpot-backend` | 9001 | the RPC API (`/api/rpc/command/*`), with the `enable-access-tokens` flag |
| `penpot-exporter` | 9002 | asset and SVG export |

`dendrite motor health penpot` probes all three ports and lists each as UP or DOWN.

## Create the token

1. Open `http://localhost:9000`, sign in (`PENPOT_USER_EMAIL` / `PENPOT_USER_PASSWORD` in `.env`).
2. Profile > Access tokens > New token. Copy it when it is shown: Penpot never shows it again.
3. Put it in the empty slot: `dendrite soma secrets set PENPOT_ACCESS_TOKEN <token>`. `secrets ensure --generate` never invents this value, because only Penpot can issue it.

The client in `pkg/ui/penpot/rpc` reads `PENPOT_ACCESS_TOKEN` (and `PENPOT_BACKEND_URL`, default `http://127.0.0.1:9001`). It sends `Authorization: Token <token>` and never writes the token into an error.

## Apply the flags to a running stack

A restart keeps the old environment (DEN-441), so stop and start:

```bash
cp stacks/compose.penpot.yaml ~/.dendrite/stacks/compose.penpot.yaml   # motor reads this copy
dendrite motor stop penpot && dendrite motor start penpot
docker inspect dendrite-penpot-backend --format '{{range .Config.Env}}{{println .}}{{end}}' | rg PENPOT_FLAGS
```
