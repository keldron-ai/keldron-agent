FROM python:3.12-slim

RUN pip install --no-cache-dir pymodbus==3.12.1

WORKDIR /app
COPY modbus_server.py config.json ./

EXPOSE 1502

CMD ["python", "modbus_server.py"]
