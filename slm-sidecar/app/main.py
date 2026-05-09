"""FastAPI app for the Ekō SLM sidecar.

Exposes /healthz, /predict, /predict_batch. Loads the openai/privacy-filter
base model and the iamSamurai/privacy-filter-nigeria LoRA adapter on startup
inside the lifespan handler, runs a warmup inference before serving traffic,
and reports model_loaded=true via /healthz only after warmup completes.
"""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from typing import AsyncIterator

from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse

from app.inference import InferenceEngine, build_engine_from_env
from app.schemas import (
    HealthResponse,
    PredictBatchItem,
    PredictBatchRequest,
    PredictBatchResponse,
    PredictRequest,
    PredictResponse,
    Span,
)

logger = logging.getLogger("eko.slm")


class AppState:
    engine: InferenceEngine | None = None
    ready: bool = False


state = AppState()


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    engine = build_engine_from_env()
    engine.load()
    engine.warmup()
    state.engine = engine
    state.ready = True
    logger.info("sidecar ready")
    try:
        yield
    finally:
        state.ready = False


app = FastAPI(title="Ekō SLM Sidecar", version="0.1.0", lifespan=lifespan)


@app.get("/healthz")
async def healthz() -> JSONResponse:
    if not state.ready:
        return JSONResponse(
            status_code=503,
            content=HealthResponse(status="warming", model_loaded=False).model_dump(),
        )
    return JSONResponse(
        status_code=200,
        content=HealthResponse(status="ok", model_loaded=True).model_dump(),
    )


def _require_engine() -> InferenceEngine:
    if state.engine is None or not state.ready:
        raise HTTPException(status_code=503, detail="model not ready")
    return state.engine


def _to_api_spans(raw_spans) -> list[Span]:
    return [
        Span(label=s.label, text=s.text, start=s.start, end=s.end, score=float(s.score))
        for s in raw_spans
    ]


@app.post("/predict", response_model=PredictResponse)
async def predict(req: PredictRequest) -> PredictResponse:
    engine = _require_engine()
    spans = engine.predict(req.text)
    return PredictResponse(spans=_to_api_spans(spans))


@app.post("/predict_batch", response_model=PredictBatchResponse)
async def predict_batch(req: PredictBatchRequest) -> PredictBatchResponse:
    engine = _require_engine()
    batches = engine.predict_batch(req.texts)
    return PredictBatchResponse(
        results=[PredictBatchItem(spans=_to_api_spans(b)) for b in batches]
    )
