# End-to-end checks against an isolated Plane

Dendrite's unit tests never reach the Plane you work in. When a change needs a **real** Plane (the board answers
differently from any fixture), run it against a second, separate copy.

```bash
tests/e2e/plane-test.sh up         # start it: its own network, volumes and ports (API 8190, UI 8191, DB 5444, Redis 6390)
tests/e2e/plane-test.sh bootstrap  # a workspace and an API token; variables land in tests/e2e/.state/env
tests/e2e/lifecycle.sh             # create, claim, lock, comment, decision, plan, finish (blocked and done)
tests/e2e/plane-test.sh down       # stop it, keep its data (reset also removes the data)
```

What keeps it apart from the live stack (`stacks/compose.plane.yaml`):

- every command runs as `docker compose -p dendrite-test`, and the compose file sets no `container_name`, so nothing
  can collide with the `dendrite-plane-*` containers;
- the ports are the live ones plus 100, and `plane-test.sh` refuses a port, or a `PLANE_BASE_URL`, on 8090, 8091, 5434
  or 6380;
- `lifecycle.sh` uses a throw-away `DENDRITE_HOME` and refuses the real `~/.dendrite`;
- the model is never asked: the gateway URL is pointed at a closed port, and the scenarios use tickets without
  `[AC-n]` criteria.

To run anything else against it: `source tests/e2e/.state/env` in a shell where you want the test Plane, nowhere else.
