FROM python:3.12-slim

RUN pip install --no-cache-dir flask==3.1.3

RUN useradd --create-home slurm

WORKDIR /app
COPY mock_slurmrestd.py .
RUN chown -R slurm:slurm /app

EXPOSE 6820

USER slurm
CMD ["python", "mock_slurmrestd.py"]
