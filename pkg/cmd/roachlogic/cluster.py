# Copyright 2026 The Cockroach Authors.
#
# Use of this software is governed by the CockroachDB Software License
# included in the /LICENSE file.

import getpass
import os
import secrets
import subprocess

import psycopg


class RoachprodError(Exception):
    """Raised when a roachprod CLI command fails."""

    def __init__(self, cmd, returncode, stderr):
        self.cmd = cmd
        self.returncode = returncode
        self.stderr = stderr
        super().__init__(
            f"roachprod command failed (exit {returncode}): {' '.join(cmd)}\n{stderr}"
        )


def _run_roachprod(*args):
    """Run a roachprod CLI command and return stdout.

    Raises RoachprodError on non-zero exit.
    """
    cmd = ["roachprod"] + list(args)
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RoachprodError(cmd, result.returncode, result.stderr)
    return result.stdout.strip()


def _generate_cluster_name(username=None):
    """Generate a unique cluster name with the user's prefix."""
    if username is None:
        username = os.environ.get("ROACHPROD_USER") or getpass.getuser()
    suffix = secrets.token_hex(2)
    return f"{username}-rl-{suffix}"


class Cluster:
    """Wraps a roachprod cluster with methods for lifecycle and SQL access.

    Do not construct directly. Use roachlogic.create() or roachlogic.attach().
    """

    def __init__(self, name, *, owned=True):
        self.name = name
        self._owned = owned

    def start(self, *, secure=True, extra_args=None):
        """Start cockroach on all nodes."""
        cmd = ["start", self.name]
        if secure:
            cmd.append("--secure")
        else:
            cmd.append("--insecure")
        if extra_args:
            for arg in extra_args:
                cmd.extend(["--args", arg])
        _run_roachprod(*cmd)

    def stop(self, *, sig=9, wait=True):
        """Stop cockroach on all nodes."""
        cmd = ["stop", self.name, f"--sig={sig}"]
        if wait:
            cmd.append("--wait")
        _run_roachprod(*cmd)

    def destroy(self):
        """Destroy the cluster and release cloud resources."""
        _run_roachprod("destroy", self.name)

    def pgurl(self, node=1, *, external=True, secure=True):
        """Return a postgres:// connection URL for the given node.

        By default returns external URLs (reachable from this machine)
        for a secure cluster.
        """
        cmd = ["pgurl", f"{self.name}:{node}"]
        if external:
            cmd.append("--external")
        if secure:
            cmd.append("--secure")
        else:
            cmd.append("--insecure")
        raw = _run_roachprod(*cmd)
        # roachprod wraps URLs in single quotes; strip them.
        return raw.strip("' \n")

    def connect(self, node=1, *, external=True, secure=True, autocommit=True):
        """Open a psycopg3 connection to the given node.

        Returns an open psycopg.Connection. The caller is responsible
        for closing it.
        """
        url = self.pgurl(node=node, external=external, secure=secure)
        return psycopg.connect(url, autocommit=autocommit)

    def sql(self, query, node=1, *, external=True, secure=True):
        """Execute a SQL query and return all rows.

        Opens a connection, executes, fetches, and closes. For repeated
        queries, use connect() instead to reuse the connection.
        """
        conn = self.connect(node=node, external=external, secure=secure)
        try:
            cur = conn.execute(query)
            return cur.fetchall()
        finally:
            conn.close()

    def run(self, command, *, nodes=None):
        """Run a shell command on cluster nodes via roachprod run.

        Args:
            command: The shell command to run.
            nodes: Node selector (e.g., "1", "1-3", "1,3,5").
                   Defaults to all nodes.

        Returns:
            stdout of the command.
        """
        target = self.name
        if nodes is not None:
            target = f"{self.name}:{nodes}"
        return _run_roachprod("run", target, command)

    def __repr__(self):
        return f"Cluster({self.name!r})"

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if self._owned:
            self.destroy()
        return False
