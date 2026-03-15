#!/usr/bin/env python3
"""Mock Slurm REST API (slurmrestd) for M9 adapter integration tests.

Implements:
- GET /slurm/v0.0.40/jobs
- GET /slurm/v0.0.40/nodes

Pre-configured with 3 RUNNING jobs across 4 nodes.
Accepts X-SLURM-USER-TOKEN for auth (any non-empty token).
"""

import json
import time
from flask import Flask, request

app = Flask(__name__)

# 3 RUNNING jobs across 4 nodes
JOBS = {
    "jobs": [
        {
            "job_id": 100,
            "job_state": "RUNNING",
            "nodes": "gpu-node-[01-02]",
            "tres_alloc_str": "gres/gpu=4",
            "tres_per_node": "gres/gpu=2",
            "time_limit": 60,
            "start_time": int(time.time()) - 300,
            "user_name": "user1",
            "partition": "gpu",
            "name": "job1",
        },
        {
            "job_id": 101,
            "job_state": "RUNNING",
            "nodes": "gpu-node-03",
            "tres_alloc_str": "gres/gpu=4",
            "tres_per_node": "gres/gpu=4",
            "time_limit": 120,
            "start_time": int(time.time()) - 600,
            "user_name": "user2",
            "partition": "gpu",
            "name": "job2",
        },
        {
            "job_id": 102,
            "job_state": "RUNNING",
            "nodes": "gpu-node-04",
            "tres_alloc_str": "gres/gpu=8",
            "tres_per_node": "gres/gpu=8",
            "time_limit": 240,
            "start_time": int(time.time()) - 120,
            "user_name": "user3",
            "partition": "gpu",
            "name": "job3",
        },
    ]
}

NODES = {
    "nodes": [
        {"name": "gpu-node-01", "state": "ALLOCATED"},
        {"name": "gpu-node-02", "state": "ALLOCATED"},
        {"name": "gpu-node-03", "state": "ALLOCATED"},
        {"name": "gpu-node-04", "state": "ALLOCATED"},
    ]
}


@app.route("/slurm/v0.0.40/jobs", methods=["GET"])
def jobs():
    token = request.headers.get("X-SLURM-USER-TOKEN", "")
    if not token:
        return {"error": "auth required"}, 401
    return JOBS


@app.route("/slurm/v0.0.40/nodes", methods=["GET"])
def nodes():
    token = request.headers.get("X-SLURM-USER-TOKEN", "")
    if not token:
        return {"error": "auth required"}, 401
    return NODES


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=6820)
