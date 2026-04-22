# Copyright 2026 The Cockroach Authors.
#
# Use of this software is governed by the CockroachDB Software License
# included in the /LICENSE file.

"""roachlogic: a Python API for roachprod clusters and CockroachDB connections.

Usage:
    from roachlogic import create

    db = create(nodes=4)
    conn = db.connect()
    conn.execute('SELECT 1')
    conn.close()
    db.destroy()

Or as a context manager:
    with create(nodes=4) as db:
        rows = db.sql('SELECT 1')
"""

from roachlogic.cluster import Cluster, RoachprodError, _generate_cluster_name, _run_roachprod

__all__ = ["create", "attach", "Cluster", "RoachprodError"]


def create(
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
):
    """Create a new roachprod cluster, stage cockroach, and start it.

    Args:
        nodes: Number of nodes (default 4).
        name: Full cluster name. Auto-generated if None.
        username: Username prefix. Detected from ROACHPROD_USER or login if None.
        lifetime: Cluster lifetime (default "12h").
        cloud: Cloud provider (default "gce").
        version: Cockroach version/SHA to stage. None means latest edge build.
        secure: Start in secure mode (default True).
        start: Whether to stage + start cockroach (default True).
            Set to False to create VMs only.
        extra_args: Extra arguments passed to cockroach start.

    Returns:
        A Cluster instance, ready for connect() if start=True.

    Raises:
        RoachprodError: If any roachprod command fails.
    """
    if name is None:
        name = _generate_cluster_name(username)

    create_cmd = [
        "create", name,
        f"--nodes={nodes}",
        f"--lifetime={lifetime}",
        f"--clouds={cloud}",
    ]
    _run_roachprod(*create_cmd)

    cluster = Cluster(name, owned=True)

    if start:
        try:
            # Stage cockroach binary.
            stage_args = ["stage", name, "cockroach"]
            if version is not None:
                stage_args.append(version)
            _run_roachprod(*stage_args)

            # Start cockroach.
            cluster.start(secure=secure, extra_args=extra_args)
        except Exception:
            # Clean up on failure so the user doesn't leak cloud resources.
            try:
                cluster.destroy()
            except Exception:
                pass
            raise

    return cluster


def attach(name):
    """Attach to an existing roachprod cluster by name.

    The returned Cluster will NOT be destroyed by the context manager
    __exit__. Call destroy() explicitly if needed.

    Args:
        name: The full cluster name (e.g., "michae2-mytest").

    Returns:
        A Cluster instance.
    """
    return Cluster(name, owned=False)
