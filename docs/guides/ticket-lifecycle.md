# Ticket lifecycle

Plane owns ticket state. Every change to a project follows one ticket through these states:

| State | Plane group | Meaning | Set with |
|---|---|---|---|
| Backlog | backlog | Known, not scheduled; where a ticket returns when work stops | `dendrite chronos ticket set DEN-n backlog` |
| To Do | unstarted | Scheduled and ready | `... todo` |
| In Progress | started | Someone is working on it now | `... "in progress"` |
| Blocked | started | Work stopped; a named dependency or decision must clear first | `... blocked` |
| To Verify | started | Finished by the agent, waiting for human sign-off | `... "to verify"` |
| Done | completed | Verified by a human | `... done` |
| Cancelled | cancelled | Will not be done | `... cancelled` |

The same states are available to agents through the `dendrite_plane_ticket_set` MCP tool.

## The loop

1. Pull the next ticket (`dendrite chronos next`) or create one (`dendrite chronos create`).
2. Design the plan, then move the ticket to **In Progress** (`dendrite work new <ticket>` creates the branch copy).
3. Commit with `dendrite quantal`, which references the ticket.
4. When the plan ends: **To Verify** (finished), **Blocked** (waiting on something, say what), **Cancelled** (dropped), or **Backlog** (parked).

Agents never move a ticket straight to Done.

## Scheduling rules

- `chronos next` never selects Blocked or Cancelled tickets.
- A Blocked ticket keeps holding back the tickets that depend on it.
- A Cancelled ticket no longer holds back its dependents.
- To Verify counts as finished when deciding whether a dependent is unblocked.

## Boards

`dendrite chronos states reconcile --all` brings every project's board to these states (it adds Blocked where missing and never touches states of your own).
