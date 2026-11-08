# Capacity pools

Pools maintain disposable guests from captured machine definitions. The existing
per-user daemon owns their requests, leases and allocation ledger. Registration
creates no guests; explicit activation enables maintenance. Preserve the user's
placement directive and unrelated definitions.

## Define and activate

```toml
[machine.worker]
provider = "docker"
kind = "container"
os = "alpine"
image = "alpine:3.21"
needs = { kind = "container" }
resources = { cpus = 1, memory = "256MiB" }

[machine.worker.functions.work]
run = 'printf "%s\n" "$1"; exit "${2:-0}"'

[izmachine.pool.build]
template = "machine.worker"
min_idle = 1
max_instances = 2
functions = ["work"]
```

```sh
izmachine validate --format json
izmachine daemon start --format json
izmachine pool register build --format json
izmachine pool up build --dry-run --format json
izmachine pool up build --format json
izmachine pool submit build work --request-id build-42 --format json -- hello 37
izmachine pool request build-42 --format json
```

Use a caller-generated request ID. Identical resubmission returns its existing
state; conflicting contents fail. Acceptance is durable, and a disconnected
client does not cancel work. Inspect the request until terminal; completed
functions expose `exit_code` and bounded `output`. Only approved guest functions
run. Arguments follow `--`; requests do not carry arbitrary host shell code or
credentials. An uncertain result remains occupied and is not replayed.

## Acquire and release

```sh
izmachine pool acquire build --request-id worker-42 --format json
izmachine pool request worker-42 --format json
izmachine pool renew worker-42 --generation <generation> --format json
izmachine direct run <guest> --provider docker --remote <endpoint> \
  --lease worker-42@<generation> --format json -- uname -a
izmachine pool release worker-42 --generation <generation> --format json
```

Wait for `leased`, then use the generation and allocation returned by the owner.
Each instance holds one lease. Direct and file-scoped mutations on a pool member
require its current claim. Low CPU, expiry and a lost response do not prove that
guest work ended. `izmachine pool cancel <request-id>` cancels queued demand or
requests confirmed termination of active work.

## Drain and recover

`izmachine pool down build` rejects new demand and drains idle capacity. Existing
leases retain their revision and accounting. Release defaults to destruction;
`lifecycle.on_down = "stop"` retains the guest and its budget. Destroy a retained,
unoccupied allocation with its ID, current generation and `--yes`.

An Incus machine can declare
`lifecycle = { on_up = "restore", snapshot = "clean" }`. The pool captures its
baseline after bootstrap and requires an identity-preserving reset before reuse.
Unsupported pool reset refuses; ordinary manual lifecycle fallback is separate.

Inspect `izmachine pool status build --format json` for unknown allocations,
failures and admission diagnostics. Recovery verifies provider identity and
generation without replaying work. Use `--provider-remote` only when the selected
alias proves the same resource domain. Cancel occupied work before recovery.
Never unlock an unrelated guest or delete a same-name replacement.

After demand and resources drain, unregister the disabled pool. Image pruning
removes only exact owned, unused prepared images. Template pruning removes an
unreferenced captured revision. Selecting a revision requires a disabled pool;
active leases keep their captured revision.

## Owners, limits and capabilities

Use `izmachine pool endpoint register <name>` for an existing local or SSH remote.
Select that owner with `--remote <name>`; `izmachine pool status --fleet` reads
explicit endpoints. Transport uses authenticated SSH and private local IPC,
with one receiving owner and no recursive peer discovery or public job listener.
Aliases of one physical Docker engine or Incus server share admission accounting.

Trusted ceilings live in the owner's user configuration under `capacity`.
Project preferences cannot raise them. Omission inherits; explicit zero prohibits
capacity. Pending, stopped, uncertain and deleting allocations remain charged.
These cooperative limits do not impose a global ceiling on unrelated users or
native toolkit operations. `pool_access` restricts pools and operations within
the existing OS-account trust boundary; project configuration cannot widen it.

Native daemon and lease flows support Linux, macOS and Windows. Certified pool
guests are Docker Linux containers and Incus Linux VMs. Inspect each provider's
`pool` capability object in `izmachine help --json`; ordinary provider or plugin
verbs do not imply pool support. The native protocol does not include GitHub
runner registration, a CI adapter or automatic owner failover.
