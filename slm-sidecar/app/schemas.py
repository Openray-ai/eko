from pydantic import BaseModel, Field


class Span(BaseModel):
    label: str
    text: str
    start: int = Field(..., description="UTF-8 byte offset of span start")
    end: int = Field(..., description="UTF-8 byte offset of span end (exclusive)")
    score: float


class PredictRequest(BaseModel):
    text: str


class PredictResponse(BaseModel):
    spans: list[Span]


class PredictBatchRequest(BaseModel):
    texts: list[str]


class PredictBatchItem(BaseModel):
    spans: list[Span]


class PredictBatchResponse(BaseModel):
    results: list[PredictBatchItem]


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
