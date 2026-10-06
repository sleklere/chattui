# Isolated protocol, terminal and load checks

Use Go 1.26.1 or newer and Docker with access to a local daemon. The commands below use a new results directory under `/tmp/opencode`. Substitute another **absolute** directory that does not yet exist; the runner creates it with mode 0700. Do not reuse a results directory. No `.env` is read by the runner. It generates database, JWT and fixture credentials in memory. Keep result directories private even though credentials, raw terminal input and terminal frames are not written there. Redirect verbose commands to a private log outside the repository.

```sh
go run -mod=readonly ./cmd/check --repository "$PWD" --results /tmp/opencode/check-e2e-01
```

This default is mandatory protocol **and** two real TUI clients on fresh PostgreSQL and Badger environments. Both jobs use one image ID built before the checks. The protocol check creates accounts, rooms and DMs, checks live recipients and duplicates, disconnected recipient history, server stop/start and persisted history. The TUI check uses two isolated PTYs, checks the rendered viewport, room/DM send and receive, reconnect, history and clean exit. `--mode protocol` explicitly runs protocol only; it does not claim TUI passed. A failed or skipped stage returns nonzero. Each job retains its own protocol trace, TUI steps, environment snapshots and redacted logs. The default client binary is built into the private results directory before the server starts; `--client-binary /absolute/path` uses a prebuilt executable instead. `--seed` controls logical fixture bodies; physical account/room names include random collision-avoidance suffixes.

Run the bounded room and DM load checks separately. They are **not** TUI checks:

```sh
go run -mod=readonly ./cmd/check --repository "$PWD" --results /tmp/opencode/check-room-01 --mode compare --profile room --repeats 2 --steps 2 --rate 2 --max-rate 4 --duration 2s --max-duration 4s
go run -mod=readonly ./cmd/check --repository "$PWD" --results /tmp/opencode/check-dm-01 --mode compare --profile dm --repeats 2 --steps 2 --rate 2 --max-rate 4 --duration 2s --max-duration 4s
```

`compare` requires both backends and runs PostgreSQL then Badger in the first repetition, Badger then PostgreSQL in the next. Each step gets a **fresh** environment, the same seed and a pinned shared image ID; `server-image.json` records the running container's ID and the expected image ID. Room load defaults to one room containing all users. To vary room targets and recipient sets, pass `--users 4 --rooms 2 --members-per-room 2`: each room has two seed-selected members; messages alternate rooms and cycle senders within each room. The number of rooms is bounded to 16 and cannot exceed the scheduled measured messages; members per room are bounded to 2..users. Zero-valued room flags select the defaults. Room settings are rejected for the DM profile. Rate and duration double only up to their explicit caps. Steps stop at the first failure. This is a whole-server offered-load comparison on this host, not a storage-engine benchmark or capacity certification. The runner rejects users outside 2..32, odd DM user counts, rates over 100/s, durations over two minutes, more than eight steps, and more than 1,000 scheduled messages per phase. `--mode load --backend postgres|badger --profile room|dm` runs a single backend. `--warmup` runs outside measured counters but verifies its own recipients, duplicates and disconnects before measurement; `--drain` waits for receives. A scheduler-late step or disconnected socket fails. No rate is auto-tuned. Both server and PostgreSQL containers default independently to `--cpus 2 --memory 2147483648 --memory-swap 2147483648`; `--server-cpu`, `--postgres-cpu`, `--server-memory` and `--postgres-memory` override them. Docker inspect validates the effective limits. The generator runs outside those containers on the same host.

`plan.json` records the seed, parameters and ordered jobs. Each load job writes a summary (`load-*.json`) with offered, sent, observed, correlated, duplicates, missing, wrong recipient, mismatches, send errors, disconnects, latency p50/p95/p99 and scheduler delay. The runner uses its own monotonic send-to-receive clock; WebSocket has no delivery ACK. `container-samples.json` holds Docker CPU, memory and cgroup throttling samples; `environment/state-*.json` includes restart/OOM state and inspected limits. `generator.json` holds per-job process CPU time, process peak RSS (process lifetime), peak Go heap and goroutines. Container CPU stats use Docker's reporting window; sampling and other host workloads add noise. Build, migrations, warmup and cleanup are outside the measured message interval. Both backends use PostgreSQL for metadata. Message history under load checks only the latest 100 per chat because the REST API has no cursor for older records. Room and DM fixture names do not need to match byte-for-byte across independent runs; logical bodies and schedule seed do.

Load stores error counts and summary by default. `--full-trace` also stores every expected and observed message (non-secret fixture bodies). `--diagnostic` performs at most one separate rerun after a load failure, with the same seed and step settings; it retains the original failure status and files. These options can use more disk space. Source log files are retained under the private job directory. Never place credentials in command-line flags or a trace.

For an existing loopback server, only a partial, explicitly opted-in **load** run is supported:

```sh
go run -mod=readonly ./cmd/check --repository "$PWD" --results /tmp/opencode/check-existing-01 --mode load --backend postgres --profile room --existing-http http://127.0.0.1:8080 --existing-ws ws://127.0.0.1:8080/api/v1/ws
```

The runner creates fixture accounts and messages there; it never stops, deletes or inspects that server and cannot establish a fresh dataset, inspect its limits or measure its container resources. Use a disposable server you own. `plan.json` marks `existing_server_partial: true`. Existing server mode cannot claim restart, lifecycle or TUI results. Wrong endpoint schemes, non-loopback hosts, missing paired endpoints and unsupported modes fail before any job starts.

Validation happens before building or starting containers. On success the process prints `PASS` and exits zero. A failed assertion, incomplete sampling, image-ID mismatch or cleanup failure returns nonzero. Successful Docker cleanup removes only resources labeled for the run; shared image cleanup first verifies its owner label, pinned ID and sole tag. Failed ownership checks leave the image for manual inspection rather than deleting a foreign reference. No runner path invokes Docker prune, `docker compose down`, `git clean` or page-cache flushing.

Focused tests (Docker is not required; real Docker checks are not skipped into success by the CLI):

```sh
go test -mod=readonly ./cmd/check ./internal/checkenv -count=1
go vet -mod=readonly ./cmd/check ./internal/checkenv
```
