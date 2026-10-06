# Badger message storage

Badger is an embedded database that can be used in production. It does not
provide distributed replication, routing or failover. This implementation is a
single-server experiment for learning and measurement, not a production
recommendation. PostgreSQL remains the default backend. Elasticsearch and search
are outside this experiment.

See [experiment results](BADGER_RESULTS.md) for the completed checks, load
comparison and limitations, and [runner documentation](TESTING.md) for all options.

## Select a backend

| Variable | Default | Purpose |
| --- | --- | --- |
| `MESSAGE_STORE` | `postgres` | `postgres` or `badger`; other values abort startup |
| `BADGER_PATH` | `data/messages.badger` | Badger directory for a local server |

With the existing PostgreSQL and JWT configuration in place:

```bash
MESSAGE_STORE=badger go run ./cmd/server
```

Or use Compose:

```bash
MESSAGE_STORE=badger docker compose up -d --build server
```

Compose sets `BADGER_PATH=/data/messages.badger` and persists `/data` in the
`badgerdata` volume. The usual `JWT_SECRET` configuration is still required.
Keep using the same `MESSAGE_STORE` value when recreating the server. To return
to PostgreSQL:

```bash
MESSAGE_STORE=postgres docker compose up -d server
```

Each backend has independent history and message IDs. Existing PostgreSQL
messages are neither copied nor deleted when Badger is selected; they are not
visible through Badger history. Returning to PostgreSQL hides the Badger
messages, without copying or deleting them. Do not merge histories by message
ID: the two ID sequences can overlap. Inbox previews remain in PostgreSQL and
may refer to messages from the previously selected backend.

## Code and data flow

- `cmd/server/internal/messagestore/store.go` defines the message interfaces;
  the existing sqlc `Queries` also implements them.
- `badger.go` persists and lists messages. `metadata.go` validates user/chat
  references and resolves current usernames through PostgreSQL.
- `room.Service` and `conversation.Service` can select a message adapter while
  retaining PostgreSQL for room membership and conversation identity.
- `cmd/server/main.go` selects one adapter at startup and closes Badger on exit.
  HTTP and WebSocket message paths use the same services.

There are no authoritative dual writes: in Badger mode, message bodies are
stored in Badger, not in PostgreSQL's `messages` table. Inbox previews are still
derived PostgreSQL data, as before. Badger mode still needs PostgreSQL available
for metadata operations.

## Keys, IDs and history

The v1 key format is:

```text
meta/v1/last-message-id
message/v1/ + r + uint64(roomID)         + uint64(messageID)
message/v1/ + c + uint64(conversationID) + uint64(messageID)
```

All integers in keys and the counter use fixed-width, big-endian encoding.
Numeric ordering therefore matches key ordering, including beyond single-digit
IDs. `r` and `c` separate room history from direct-message history.

Values are JSON records containing the message ID, target, sender ID, body and
UTC timestamp. They do not contain a username snapshot; room history looks up
the current username once per distinct sender per request.

A process-local mutex serializes writers. A single Badger transaction increments
the global message counter and inserts the message. Failed transactions do not
consume an ID; restart resumes the persisted counter. IDs are positive `int64`
values, matching the existing API types.

History reads iterate the requested prefix in reverse, up to the requested
limit. This returns append order, newest ID first. PostgreSQL history continues
to order by `created_at DESC`. Neither adapter adds cursor pagination here; the
existing limit-based API is unchanged. A system-clock adjustment can make
Badger's append ordering differ from timestamp ordering.

## Consistency limits

PostgreSQL mode retains the existing transaction covering conversation creation
and its first message. Badger and PostgreSQL cannot share that transaction.
In Badger mode, sending a DM:

1. Creates or finds the conversation in PostgreSQL, upserts both inbox cursors
   and commits that transaction.
2. Validates metadata references, then commits the message and ID counter in
   Badger.
3. Publishes the message event only after the Badger write succeeds.

A failed write can leave an empty conversation and empty inbox cursors. Retrying
uses the same conversation. A conversation-created event may have been emitted,
but no message event is emitted for a failed message write.

A crash after a successful message commit but before event processing can leave
a durable message without live delivery or an updated inbox preview. The event
bus remains in-memory; there is no durable outbox or replay worker in this change.
A client retry is not deduplicated and may create another message. These are
separate concerns from Badger's on-disk recovery.

Metadata validation is an existence check, not a cross-database foreign key or
authorization system. It does not change the application's existing permission
checks. Concurrent deletion can happen after validation. PostgreSQL deletions
also do not cascade into Badger: deleting users, rooms or conversations can leave
orphaned message records, and room history can fail to resolve a deleted sender.
Do not use metadata deletion/reset as a message-retention mechanism in this mode.

## Persistence and operations

- Badger opens with `SyncWrites=true`, so successful writes request a disk flush.
  This has a latency cost. The load comparison measures the whole server;
  it does not isolate Badger throughput.
- New directories use mode `0700`. Existing directory permissions and volume
  access must be managed by the operator. Storage is not encrypted by this code.
- Badger locks its directory. Only one process may open it for writing. Do not
  point multiple server instances at this directory or share it to simulate a
  distributed database.
- Stop the server before copying its Badger directory. For a coherent backup,
  stop all application writers and back up both PostgreSQL and the complete
  Badger directory from that stopped state. Restore them as a pair. Do not use
  an old Badger directory with a freshly reset PostgreSQL database whose IDs may
  now refer to different users or chats.
- `docker compose down` retains named volumes. `docker compose down -v` destroys
  both PostgreSQL and Badger volumes.
- No import/export, online backup command, retention/deletion API, distributed
  delivery or database migration tool is added here.

## Verification

```bash
# Disk persistence, history ordering/isolation, concurrent IDs, failure handling,
# exclusive directory ownership, and recovery after exit without Close.
go test -race ./cmd/server/internal/messagestore

# Real PostgreSQL metadata + Badger service integration; requires Docker.
go test -v ./cmd/server/internal/room -run 'Test(Badger|PostgresDirect)' -count=1

# Full existing suite, including PostgreSQL integration tests.
go test ./...
```

The integration tests exercise rooms and DMs, check that Badger-mode sends leave
PostgreSQL's message table untouched, reopen history, resolve renamed users,
reject missing metadata, and retry a failed first DM. The subprocess recovery
test exits without closing Badger after a successful write; it does not simulate
power loss or disk failure. Existing integration tests skip when Docker is
unavailable, so check the verbose output before claiming they ran.
