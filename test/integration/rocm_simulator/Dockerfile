# Optional: ROCm mock as container for agent-in-Docker integration tests.
# For host-based tests, use mock_rocm_smi.sh directly.
FROM alpine:3.20

COPY mock_rocm_smi.sh /usr/local/bin/rocm-smi
RUN chmod +x /usr/local/bin/rocm-smi

# Default: just run the mock (prints JSON and exits)
# When used as rocm_smi_path, agent will exec this and parse output
ENTRYPOINT ["/usr/local/bin/rocm-smi"]
