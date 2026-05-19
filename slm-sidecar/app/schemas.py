from pydantic import BaseModel, Field, field_validator


MAX_TEXT_BYTES = 16_384
MAX_BATCH_ITEMS = 32


class Span(BaseModel):
    label: str
    text: str
    start: int = Field(..., description="UTF-8 byte offset of span start")
    end: int = Field(..., description="UTF-8 byte offset of span end (exclusive)")
    score: float


class PredictRequest(BaseModel):
    text: str

    @field_validator("text")
    @classmethod
    def text_within_byte_limit(cls, value: str) -> str:
        if len(value.encode("utf-8")) > MAX_TEXT_BYTES:
            raise ValueError(f"text must be <= {MAX_TEXT_BYTES} UTF-8 bytes")
        return value


class PredictResponse(BaseModel):
    spans: list[Span]


class PredictBatchRequest(BaseModel):
    texts: list[str] = Field(..., min_length=1, max_length=MAX_BATCH_ITEMS)

    @field_validator("texts")
    @classmethod
    def texts_within_byte_limit(cls, value: list[str]) -> list[str]:
        for text in value:
            if len(text.encode("utf-8")) > MAX_TEXT_BYTES:
                raise ValueError(f"each text must be <= {MAX_TEXT_BYTES} UTF-8 bytes")
        return value


class PredictBatchItem(BaseModel):
    spans: list[Span]


class PredictBatchResponse(BaseModel):
    results: list[PredictBatchItem]


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
