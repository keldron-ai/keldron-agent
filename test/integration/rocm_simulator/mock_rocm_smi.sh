#!/bin/sh
# Mock rocm-smi for M9 ROCm adapter integration tests.
# Mimics: rocm-smi --showtemp --showuse --showpower --json
# Returns 4 simulated AMD GPUs with fixed metrics.
cat <<'EOF'
{"gpu":[{"gpu_id":0,"temperature_edge":65,"gpu_use_percent":95,"vram_used_mb":1024,"vram_total_mb":8192,"average_socket_power":550,"throttle_status":"none","gpu_name":"AMD MI300X"},{"gpu_id":1,"temperature_edge":72,"gpu_use_percent":87,"vram_used_mb":2048,"vram_total_mb":8192,"average_socket_power":520,"throttle_status":"none","gpu_name":"AMD MI300X"},{"gpu_id":2,"temperature_edge":68,"gpu_use_percent":92,"vram_used_mb":1536,"vram_total_mb":8192,"average_socket_power":540,"throttle_status":"none","gpu_name":"AMD MI300X"},{"gpu_id":3,"temperature_edge":71,"gpu_use_percent":88,"vram_used_mb":1024,"vram_total_mb":8192,"average_socket_power":510,"throttle_status":"none","gpu_name":"AMD MI300X"}]}
EOF
