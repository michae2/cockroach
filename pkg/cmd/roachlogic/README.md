# roachlogic

A Python 3 module for managing roachprod clusters and running SQL against
CockroachDB. Wraps the `roachprod` CLI and [psycopg 3](https://www.psycopg.org/psycopg3/docs/)
into a simple API for interactive REPL use and scripting.

## Prerequisites

- `roachprod` binary on your PATH (build with `./dev build roachprod`)
- Python 3.10+
- psycopg 3 (`pip install "psycopg[binary]"`)

There is a `.venv` in the repo root with psycopg pre-installed.

## Setup

Set `PYTHONPATH` so Python can find the module:

```bash
export PYTHONPATH=/path/to/cockroach/pkg/cmd:$PYTHONPATH
```

Then import it from any Python 3 REPL or script:

```python
from roachlogic import create, attach
```

To use the repo's `.venv` directly:

```bash
PYTHONPATH=pkg/cmd .venv/bin/python3
```

## Quick start

```python
from roachlogic import create

# Create a 4-node cluster, stage cockroach, and start it.
db = create(nodes=4)

# Open a SQL connection to node 1.
conn = db.connect()
conn.execute("CREATE TABLE t (id INT PRIMARY KEY, name TEXT)")
conn.execute("INSERT INTO t VALUES (1, 'alice'), (2, 'bob')")
rows = conn.execute("SELECT * FROM t").fetchall()
print(rows)  # [(1, 'alice'), (2, 'bob')]
conn.close()

# Tear down.
db.destroy()
```

Or use a context manager so the cluster is destroyed automatically:

```python
with create(nodes=4) as db:
    rows = db.sql("SELECT version()")
    print(rows)
```

Attach to an existing cluster instead of creating a new one:

```python
db = attach("michae2-mytest")
conn = db.connect()
conn.execute("SELECT 1").fetchone()
conn.close()
```

---

## Module API

### `create()`

```python
create(
    nodes=4,
    *,
    name=None,
    username=None,
    lifetime="12h",
    cloud="gce",
    version=None,
    secure=True,
    start=True,
    extra_args=None,
) -> Cluster
```

Create a new roachprod cluster. By default this runs three roachprod
commands in sequence: `roachprod create`, `roachprod stage cockroach`,
and `roachprod start`. The returned `Cluster` is immediately ready for
`connect()`.

If staging or starting fails after the VMs have been created, the cluster
is automatically destroyed to avoid leaking cloud resources.

**Arguments:**

| Argument | Type | Default | Description |
|---|---|---|---|
| `nodes` | `int` | `4` | Number of cluster nodes. |
| `name` | `str \| None` | `None` | Full cluster name. If `None`, auto-generated as `{username}-rl-{4 hex chars}`. |
| `username` | `str \| None` | `None` | Username prefix for the cluster name. Detected from `ROACHPROD_USER` env var or OS login if `None`. |
| `lifetime` | `str` | `"12h"` | How long before roachprod garbage-collects the cluster (e.g., `"12h"`, `"3d"`). |
| `cloud` | `str` | `"gce"` | Cloud provider: `"gce"`, `"aws"`, `"azure"`, or `"local"`. |
| `version` | `str \| None` | `None` | CockroachDB version or SHA to stage. `None` stages the latest edge build. |
| `secure` | `bool` | `True` | Start in secure (TLS) mode. Set to `False` for insecure clusters. |
| `start` | `bool` | `True` | Whether to stage a binary and start cockroach. Set to `False` to create VMs only. |
| `extra_args` | `list[str] \| None` | `None` | Extra arguments passed to `cockroach start` (e.g., `["--cache=50%"]`). |

**Returns:** A `Cluster` instance with `owned=True` (context manager will destroy it).

**Raises:** `RoachprodError` if any roachprod command fails.

---

### `attach()`

```python
attach(name: str) -> Cluster
```

Attach to an existing roachprod cluster by name. Does not create, stage,
or start anything.

**Arguments:**

| Argument | Type | Description |
|---|---|---|
| `name` | `str` | Full cluster name (e.g., `"michae2-mytest"`). |

**Returns:** A `Cluster` instance with `owned=False` (context manager will
NOT destroy it; call `destroy()` explicitly if needed).

---

### `Cluster`

Wraps a roachprod cluster. Do not construct directly; use `create()` or
`attach()`.

**Attributes:**

| Attribute | Type | Description |
|---|---|---|
| `name` | `str` | The roachprod cluster name. |

#### `Cluster.start()`

```python
db.start(*, secure=True, extra_args=None)
```

Start cockroach on all nodes. Called automatically by `create()` unless
`start=False`.

| Argument | Type | Default | Description |
|---|---|---|---|
| `secure` | `bool` | `True` | Use TLS. |
| `extra_args` | `list[str] \| None` | `None` | Extra args for `cockroach start`. |

#### `Cluster.stop()`

```python
db.stop(*, sig=9, wait=True)
```

Stop cockroach on all nodes.

| Argument | Type | Default | Description |
|---|---|---|---|
| `sig` | `int` | `9` | Signal to send (9=SIGKILL, 15=SIGTERM). |
| `wait` | `bool` | `True` | Wait for processes to exit. |

#### `Cluster.destroy()`

```python
db.destroy()
```

Destroy the cluster and release all cloud resources. This is irreversible.

#### `Cluster.pgurl()`

```python
db.pgurl(node=1, *, external=True, secure=True) -> str
```

Return a `postgres://` connection URL for the given node.

| Argument | Type | Default | Description |
|---|---|---|---|
| `node` | `int` | `1` | Node number (1-indexed). |
| `external` | `bool` | `True` | Use public IPs (needed when connecting from your laptop). Set to `False` when connecting from within the same cloud VPC. |
| `secure` | `bool` | `True` | Include TLS certificate parameters in the URL. |

**Returns:** A connection string like
`postgres://root@35.196.1.2:26257?sslmode=verify-full&sslcert=...`

#### `Cluster.connect()`

```python
db.connect(node=1, *, external=True, secure=True, autocommit=True) -> psycopg.Connection
```

Open a psycopg 3 connection to the given node. The caller is responsible
for closing the returned connection.

| Argument | Type | Default | Description |
|---|---|---|---|
| `node` | `int` | `1` | Node number (1-indexed). |
| `external` | `bool` | `True` | Use public IPs. |
| `secure` | `bool` | `True` | Use TLS. |
| `autocommit` | `bool` | `True` | Enable autocommit. When `True`, each statement runs in its own implicit transaction. When `False`, psycopg opens an implicit transaction that must be committed or rolled back. `True` is the recommended default for CockroachDB. |

**Returns:** A `psycopg.Connection` (see [Connection reference](#connection-psycopgconnection) below).

#### `Cluster.sql()`

```python
db.sql(query, node=1, *, external=True, secure=True) -> list[tuple]
```

Execute a SQL query and return all result rows. This is a convenience
method that opens a connection, executes the query, fetches all rows,
and closes the connection. For repeated queries, use `connect()` instead
to reuse the connection.

| Argument | Type | Default | Description |
|---|---|---|---|
| `query` | `str` | | SQL query to execute. |
| `node` | `int` | `1` | Node number. |
| `external` | `bool` | `True` | Use public IPs. |
| `secure` | `bool` | `True` | Use TLS. |

**Returns:** A list of tuples, one per row.

```python
db.sql("SELECT id, name FROM t WHERE id > 0")
# [(1, 'alice'), (2, 'bob')]
```

#### `Cluster.run()`

```python
db.run(command, *, nodes=None) -> str
```

Run a shell command on cluster nodes via `roachprod run`.

| Argument | Type | Default | Description |
|---|---|---|---|
| `command` | `str` | | Shell command to execute on the remote node(s). |
| `nodes` | `str \| None` | `None` | Node selector. Examples: `"1"`, `"1-3"`, `"1,3,5"`. `None` means all nodes. |

**Returns:** stdout from the remote command.

```python
db.run("df -h", nodes="1")        # disk usage on node 1
db.run("ls /mnt/data1")           # list data dir on all nodes
db.run("uname -r", nodes="1-3")   # kernel version on nodes 1-3
```

#### Context manager

`Cluster` supports the `with` statement. On exit, clusters created by
`create()` are destroyed automatically. Clusters from `attach()` are not.

```python
with create(nodes=3) as db:
    db.sql("SELECT 1")
# cluster is destroyed here

with attach("michae2-existing") as db:
    db.sql("SELECT 1")
# cluster is NOT destroyed here
```

---

### `RoachprodError`

```python
class RoachprodError(Exception)
```

Raised when a `roachprod` CLI command exits with a non-zero status.

**Attributes:**

| Attribute | Type | Description |
|---|---|---|
| `cmd` | `list[str]` | The full command that was run (e.g., `["roachprod", "create", "michae2-rl-a3f7"]`). |
| `returncode` | `int` | Exit code from the process. |
| `stderr` | `str` | The stderr output from roachprod. |

```python
try:
    db = create(nodes=4)
except RoachprodError as e:
    print(f"Command failed: {e.cmd}")
    print(f"Exit code: {e.returncode}")
    print(f"Error: {e.stderr}")
```

---

## psycopg 3 reference

The `connect()` method returns a standard
[psycopg.Connection](https://www.psycopg.org/psycopg3/docs/api/connections.html).
The `execute()` method on connections and cursors returns a
[psycopg.Cursor](https://www.psycopg.org/psycopg3/docs/api/cursors.html).
This section covers the most commonly used methods.

### Connection (`psycopg.Connection`)

Returned by `db.connect()`. Represents a live session with the database.

#### Executing queries

```python
conn = db.connect()

# execute() runs a query and returns a Cursor with the results.
cur = conn.execute("SELECT id, name FROM t WHERE id = %s", [42])
row = cur.fetchone()  # (42, 'alice')

# Use %s placeholders for parameters (never f-strings or format()).
conn.execute("INSERT INTO t VALUES (%s, %s)", [3, 'charlie'])

# For multiple parameter sets, use executemany() on a cursor.
cur = conn.cursor()
cur.executemany(
    "INSERT INTO t VALUES (%s, %s)",
    [(4, 'delta'), (5, 'echo'), (6, 'foxtrot')],
)
```

**Important:** Always use `%s` parameter placeholders, never Python string
formatting. This prevents SQL injection and ensures correct type handling.

#### Autocommit vs. transactions

By default, `connect()` opens the connection with `autocommit=True`:
each statement runs in its own implicit transaction and is committed
immediately. This is the recommended mode for CockroachDB.

To use explicit transactions, either set `autocommit=False` or use the
`transaction()` context manager:

```python
# Option 1: autocommit=False (implicit transactions).
conn = db.connect(autocommit=False)
conn.execute("INSERT INTO t VALUES (1, 'alice')")
conn.execute("INSERT INTO t VALUES (2, 'bob')")
conn.commit()   # commits both inserts
# conn.rollback() would discard both

# Option 2: transaction() context manager (recommended for explicit txns).
conn = db.connect()  # autocommit=True
with conn.transaction():
    conn.execute("INSERT INTO t VALUES (1, 'alice')")
    conn.execute("INSERT INTO t VALUES (2, 'bob')")
    # committed on exiting the block; rolled back on exception
```

#### Closing

```python
conn.close()
```

Always close connections when done. Alternatively, use as a context manager:

```python
with db.connect() as conn:
    conn.execute("SELECT 1")
# connection is closed here
```

#### Other useful connection methods

| Method | Description |
|---|---|
| `conn.commit()` | Commit the current transaction. |
| `conn.rollback()` | Roll back the current transaction. |
| `conn.transaction()` | Context manager for an explicit transaction block. |
| `conn.cursor()` | Create a new cursor (useful for `executemany()` or setting a custom `row_factory`). |
| `conn.set_autocommit(bool)` | Toggle autocommit on a live connection. |
| `conn.cancel()` | Cancel the currently executing query. |
| `conn.close()` | Close the connection. |
| `conn.closed` | `True` if the connection has been closed. |
| `conn.broken` | `True` if the connection is broken (e.g., network failure). |
| `conn.info` | Connection info object with properties like `server_version`, `host`, `port`, etc. |

### Cursor (`psycopg.Cursor`)

Returned by `conn.execute()` or `conn.cursor()`. Holds query results.

#### Fetching results

```python
cur = conn.execute("SELECT id, name FROM t ORDER BY id")

cur.fetchone()     # (1, 'alice') — next row, or None if exhausted
cur.fetchall()     # [(2, 'bob'), (3, 'charlie')] — all remaining rows
cur.fetchmany(10)  # up to 10 rows

# Cursors are iterable:
for row in conn.execute("SELECT id, name FROM t"):
    print(row)  # (1, 'alice'), (2, 'bob'), ...
```

Each row is a tuple by default. Columns are ordered as in the SELECT list.

#### Result metadata

```python
cur = conn.execute("SELECT id, name FROM t")

cur.description    # column metadata: sequence of Column objects
                   #   each has .name, .type_code, etc.
cur.rowcount       # number of rows affected by INSERT/UPDATE/DELETE,
                   #   or -1 for SELECT
cur.statusmessage  # PostgreSQL status string, e.g. "SELECT 2", "INSERT 0 1"
```

#### Streaming large results

For large result sets, use `stream()` to avoid loading everything into
memory:

```python
cur = conn.cursor()
for row in cur.stream("SELECT * FROM large_table"):
    process(row)
```

#### COPY

Use `copy()` for bulk data loading:

```python
with conn.cursor().copy("COPY t (id, name) FROM STDIN") as copy:
    copy.write_row((7, 'golf'))
    copy.write_row((8, 'hotel'))
```

#### Other useful cursor methods/attributes

| Method / Attribute | Description |
|---|---|
| `cur.execute(query, params)` | Execute a query. `params` is a sequence for `%s` placeholders. |
| `cur.executemany(query, params_seq)` | Execute a query with multiple parameter sets. |
| `cur.fetchone()` | Fetch the next row, or `None`. |
| `cur.fetchall()` | Fetch all remaining rows. |
| `cur.fetchmany(size)` | Fetch up to `size` rows. |
| `cur.stream(query, params)` | Iterate rows one at a time without buffering. |
| `cur.copy(statement)` | Start a COPY operation. |
| `cur.close()` | Close the cursor. |
| `cur.closed` | `True` if the cursor has been closed. |
| `cur.description` | Column metadata (name, type, etc.) for the last query. |
| `cur.rowcount` | Rows affected by the last command. |
| `cur.statusmessage` | PostgreSQL status string for the last command. |

### Type mapping

psycopg 3 automatically converts between PostgreSQL and Python types:

| CockroachDB / PostgreSQL | Python |
|---|---|
| `INT`, `BIGINT`, `SMALLINT` | `int` |
| `FLOAT`, `DOUBLE PRECISION` | `float` |
| `DECIMAL`, `NUMERIC` | `decimal.Decimal` |
| `TEXT`, `VARCHAR`, `CHAR` | `str` |
| `BOOL` | `bool` |
| `BYTEA` | `bytes` |
| `DATE` | `datetime.date` |
| `TIMESTAMP` | `datetime.datetime` |
| `TIMESTAMPTZ` | `datetime.datetime` (with tzinfo) |
| `INTERVAL` | `datetime.timedelta` |
| `UUID` | `uuid.UUID` |
| `JSONB`, `JSON` | `dict` or `list` (Python objects) |
| `ARRAY[...]` | `list` |
| `NULL` | `None` |

### Parameter passing

Always use `%s` placeholders and pass parameters as a sequence:

```python
# Correct: parameterized query.
conn.execute("SELECT * FROM t WHERE id = %s AND name = %s", [42, 'alice'])

# Also correct: named placeholders with a dict.
conn.execute(
    "SELECT * FROM t WHERE id = %(id)s AND name = %(name)s",
    {"id": 42, "name": "alice"},
)

# WRONG: string formatting. Never do this.
conn.execute(f"SELECT * FROM t WHERE id = {user_id}")
```

---

## Recipes

### Run a query on every node and compare results

```python
db = attach("michae2-mytest")
for node in range(1, 5):
    rows = db.sql("SELECT count(*) FROM t", node=node)
    print(f"node {node}: {rows[0][0]} rows")
```

### Stop and restart a node

```python
from roachlogic import attach
from roachlogic.cluster import _run_roachprod

db = attach("michae2-mytest")

# Stop node 3 with SIGTERM and wait.
_run_roachprod("stop", f"{db.name}:3", "--sig=15", "--wait")

# Do some work while node 3 is down...
db.sql("INSERT INTO t VALUES (100, 'during-outage')", node=1)

# Restart node 3.
_run_roachprod("start", f"{db.name}:3", "--secure")
```

### Explicit transaction with retry

```python
from psycopg.errors import SerializationFailure

conn = db.connect()
while True:
    try:
        with conn.transaction():
            conn.execute("UPDATE accounts SET balance = balance - 100 WHERE id = 1")
            conn.execute("UPDATE accounts SET balance = balance + 100 WHERE id = 2")
        break  # success
    except SerializationFailure:
        continue  # retry
```

### Bulk load with COPY

```python
import csv

conn = db.connect()
conn.execute("CREATE TABLE IF NOT EXISTS data (id INT PRIMARY KEY, value TEXT)")

with conn.cursor().copy("COPY data (id, value) FROM STDIN") as copy:
    with open("data.csv") as f:
        reader = csv.reader(f)
        for row in reader:
            copy.write_row(row)
```

### Use as a context manager for cleanup

```python
from roachlogic import create

with create(nodes=3, lifetime="1h") as db:
    conn = db.connect()
    conn.execute("CREATE TABLE t (k INT PRIMARY KEY)")
    conn.execute("INSERT INTO t SELECT generate_series(1, 1000)")
    print(db.sql("SELECT count(*) FROM t"))
    conn.close()
# cluster destroyed automatically on exit
```
