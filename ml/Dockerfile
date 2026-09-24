# ekokod ML service (03 §6). No database driver is installed — `python -m ekokod_ml.nodb` proves it (R366).
FROM python:3.12-slim AS build
COPY --from=ghcr.io/astral-sh/uv:0.12.18 /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_PYTHON_DOWNLOADS=never
WORKDIR /app
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev --no-install-project
COPY src ./src
RUN uv sync --frozen --no-dev --no-editable

FROM python:3.12-slim
RUN useradd --system --uid 10001 --home /nonexistent ml \
 && mkdir -p /var/lib/ekokod-ml && chown ml /var/lib/ekokod-ml
COPY --from=build /app/.venv /app/.venv
ENV PATH=/app/.venv/bin:$PATH PYTHONUNBUFFERED=1 ML_MODEL_DIR=/var/lib/ekokod-ml
USER ml
EXPOSE 8000
HEALTHCHECK --interval=15s --timeout=5s --retries=5 \
  CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=4)"]
CMD ["uvicorn", "--factory", "ekokod_ml.api:create_app", "--host", "0.0.0.0", "--port", "8000"]
