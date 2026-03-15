#!/usr/bin/env python3
"""Modbus TCP simulator for M9 temperature adapter integration tests.

Loads register layout from config.json (falls back to defaults if missing).
Default holding registers:
- Register 100: 225 (x 0.1 = 22.5 C inlet)
- Register 101: 342 (x 0.1 = 34.2 C outlet)

Listens on port 1502.
"""

import json
from pathlib import Path

from pymodbus.datastore import (
    ModbusDeviceContext,
    ModbusServerContext,
    ModbusSequentialDataBlock,
    ModbusSparseDataBlock,
)
from pymodbus.server import StartTcpServer

DEFAULT_REGISTERS = {100: 225, 101: 342}
DEFAULT_PORT = 1502
DEFAULT_UNIT_ID = 1


def load_config():
    """Load config.json from the same directory as this script."""
    config_path = Path(__file__).parent / "config.json"
    try:
        with open(config_path) as f:
            raw = json.load(f)
        registers = {int(k): v for k, v in raw.get("registers", {}).items()}
        port = raw.get("port", DEFAULT_PORT)
        return registers or DEFAULT_REGISTERS, port
    except (FileNotFoundError, json.JSONDecodeError, ValueError):
        return DEFAULT_REGISTERS, DEFAULT_PORT


def run():
    registers, port = load_config()

    # ModbusDeviceContext.getValues applies a +1 offset internally
    # (PDU address N maps to data-block address N+1). Clients should
    # use 0-based PDU addresses; pymodbus handles the translation.
    hr_block = ModbusSparseDataBlock(registers)

    # Separate blocks for each register type to prevent cross-contamination.
    store = ModbusDeviceContext(
        di=ModbusSequentialDataBlock(0, [0] * 100),
        co=ModbusSequentialDataBlock(0, [0] * 100),
        hr=hr_block,
        ir=ModbusSequentialDataBlock(0, [0] * 100),
    )
    context = ModbusServerContext(devices=store, single=True)

    StartTcpServer(context=context, address=("0.0.0.0", port))


if __name__ == "__main__":
    run()
