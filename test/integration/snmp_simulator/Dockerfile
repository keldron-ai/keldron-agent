FROM python:3.12-slim

RUN pip install --no-cache-dir snmpsim==1.2.1

WORKDIR /app
COPY data/ /app/data/

# snmpsim drops privileges to nobody at runtime via --process-user;
# ensure the data directory is readable by that user.
RUN chown -R nobody:nogroup /app/data
EXPOSE 1161/udp

CMD ["snmpsim-command-responder", "--data-dir=/app/data", "--agent-udpv4-endpoint=0.0.0.0:1161", "--process-user=nobody", "--process-group=nogroup"]
