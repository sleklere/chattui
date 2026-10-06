# Badger message storage experiment results

This branch implements Badger as an optional authoritative store for room and
DM messages. PostgreSQL remains the default and stores users, memberships,
conversation identities and inbox data in both modes. The experiment is not a
proposal to replace PostgreSQL in production.

[Storage design and limitations](BADGER.md) describes the implementation.
[Runner documentation](TESTING.md) covers configuration and artifacts.

## What was tested

- On-disk persistence, concurrent message IDs, history ordering and isolation,
  exclusive directory ownership, failed writes and reopen recovery.
- Real PostgreSQL metadata with Badger message storage, including unchanged
  PostgreSQL message tables, current usernames and a failed first DM retry.
- Integrated REST/WebSocket E2E on both backends: room and DM routing, recipient
  isolation, offline history, persisted history after restart and live delivery
  after restart.
- Two actual TUI clients in separate Linux PTYs on each backend: registration,
  room creation/join, live messages, reconnect, offline history, DM delivery and
  history, and clean exit. Assertions use the rendered terminal viewport and
  REST checks, not a transcript search.
- Whole-server open-loop load for room and DM profiles in fresh Docker
  environments, with verified warmup and REST history checks.

The integrated E2E passed for both backends. The full race-enabled test suite,
`go vet`, CGO-disabled builds of server/client/check, Compose configuration
validation and whitespace checks also passed. These are results of the completed
run, not guarantees about future environments.

## Load comparison setup

| Setting | Value |
| --- | --- |
| Connected users | 8 |
| Room topology | 4 rooms, 4 members per room |
| DM topology | 4 account pairs |
| Logical fixture seed | 17 |
| First step | 8 offered messages/s for 4 seconds |
| Second step | 16 offered messages/s for 8 seconds |
| Warmup / receive drain | 2 seconds / 2 seconds |
| Repetitions | 2 per backend and step, per profile |
| Order | PostgreSQL then Badger; reversed for the second repetition |
| Server limits | 2 CPU, 2 GiB memory, no additional swap |
| PostgreSQL limits | 2 CPU, 2 GiB memory, no additional swap |

There were 16 load jobs and 2 integrated E2E jobs. Each job used a fresh dataset.
Within each comparison invocation, all jobs ran the same server image pinned by
its actual Docker image ID. Effective CPU and memory limits were verified through
Docker inspect. The generator ran on the same Linux host, outside the containers.
Builds, migrations and warmup were outside the measured message interval.

## Observed results

Across the load jobs, 1,280 measured messages were sent and 3,840 recipient
deliveries were observed and correlated. Room messages had four expected
recipients, including the sender; DMs had two. There were no missing or duplicate
deliveries, wrong recipients, content/ID discrepancies, send errors or socket
disconnections. Warmup and recoverable REST history checks passed as well.

At 16 offered messages/s, send-to-receive p95 latency in milliseconds was:

| Profile | PostgreSQL, repetition 1 / 2 | Badger, repetition 1 / 2 |
| --- | --- | --- |
| Room | 2.978 / 2.930 | 2.889 / 3.290 |
| DM | 3.136 / 3.105 | 866.514 / 5.647 |

The high Badger DM p95 in the first repetition is part of the result. It was not
removed or replaced by a favorable retry. Its cause was not established. No
latency SLO was specified, so the runner's PASS means that delivery, persistence,
scheduling and resource checks succeeded; it does not certify low latency.

Container sampling completed without errors. Samples showed no increase in
throttled CPU periods between their first and last observations. Sampling is not
continuous observation and cannot rule out every resource stall.

## What these results do not establish

These short, low-rate runs did not establish a saturation point, maximum
capacity or a performance winner. Similar room latencies at this load do not
prove that the backends perform equally under stress. The DM variation also
prevents a stable latency comparison.

The runner measures the complete server, including metadata operations and
broadcasting, rather than either storage engine in isolation. Both modes still
need PostgreSQL. The generator shares host resources with the containers;
container samples include preparation and warmup, and process peak RSS covers
the generator's lifetime. Host contention and Docker reporting windows add noise.

Load history verification covers the recoverable suffix of at most 100 messages
per chat because the API has no cursor. Subprocess-exit recovery is not a
power-loss test. Distributed replication, failover, migration between backends,
a durable outbox and Elasticsearch are outside the experiment.

A further stress experiment should increase offered load within explicit bounds,
retain the first failure, and check generator lag and resource usage before
attributing a limit to storage. Longer runs and repeated tail-latency measurements
would be needed before recommending a backend on performance grounds.

## Reproduce the checks

Use a local Docker daemon and run from the repository root. Each invocation
needs a new absolute results directory. The temporary parent directory below is
private; none of its generated binaries, logs or artifacts belong in Git.

```sh
RESULTS=$(mktemp -d "${TMPDIR:-/tmp}/chattui-badger.XXXXXX")
go build -mod=readonly -o "$RESULTS/check" ./cmd/check

"$RESULTS/check" --repository "$PWD" --results "$RESULTS/e2e" --seed 17

"$RESULTS/check" --repository "$PWD" --results "$RESULTS/room" \
  --mode compare --profile room --users 8 --rooms 4 --members-per-room 4 \
  --seed 17 --rate 8 --max-rate 16 --duration 4s --max-duration 8s \
  --warmup 2s --drain 2s --steps 2 --repeats 2 --full-trace

"$RESULTS/check" --repository "$PWD" --results "$RESULTS/dm" \
  --mode compare --profile dm --users 8 \
  --seed 17 --rate 8 --max-rate 16 --duration 4s --max-duration 8s \
  --warmup 2s --drain 2s --steps 2 --repeats 2 --full-trace

go test -mod=readonly -race -count=1 ./...
go vet -mod=readonly ./...
```

The runner records the plan, summaries, image identity, resource snapshots and
traces in those private directories and checks ownership before cleanup. The
published report contains aggregate results and reproducible settings only.
Original run artifacts remain local; personal paths, credentials, account names,
container identifiers and unrelated host information are not included here.
