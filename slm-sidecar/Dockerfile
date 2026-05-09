FROM python:3.12-slim AS base

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_NO_CACHE_DIR=1 \
    HF_HUB_DISABLE_TELEMETRY=1 \
    HF_HOME=/root/.cache/huggingface \
    TRANSFORMERS_VERBOSITY=warning

# CPU-only torch wheels keep the image small. Override --build-arg TORCH_INDEX
# at build time to pull GPU wheels (e.g. https://download.pytorch.org/whl/cu121).
ARG TORCH_INDEX=https://download.pytorch.org/whl/cpu

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
    && rm -rf /var/lib/apt/lists/*

# Install dependencies from pyproject.toml so versions don't drift between the
# package manifest and the Dockerfile.
COPY pyproject.toml README.md ./
COPY app ./app
RUN pip install --upgrade pip \
    && pip install --extra-index-url ${TORCH_INDEX} .

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
