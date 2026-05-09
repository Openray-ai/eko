"""Inference for openai/privacy-filter + iamSamurai/privacy-filter-nigeria adapter.

Loads a token-classification base model and a LoRA adapter, runs inference on
text, and converts token-level predictions into character spans with UTF-8 byte
offsets. Byte offsets match the offsets used by Ekō's regex detector
(internal/core/detector/detector.go uses byte offsets in Violation.Position/End),
so the Go side can merge SLM and regex violations without converting indices.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass

# Heavy ML deps (torch, transformers, peft) are imported lazily inside the
# methods that need them. This keeps module import cheap so the byte-offset
# helpers, RawSpan, and the pure-Python span extraction / post-processing
# logic can be unit-tested without installing torch.

logger = logging.getLogger(__name__)


@dataclass
class RawSpan:
    label: str
    text: str
    start: int  # byte offset
    end: int    # byte offset
    score: float


class InferenceEngine:
    """Loads and runs the privacy-filter model + LoRA adapter.

    Single instance per process; methods are not thread-safe (uvicorn workers
    handle concurrency by running multiple processes). FastAPI's default
    single-worker dev mode is fine for testing; production deployments should
    set --workers to a small number scaled to CPU/GPU capacity.
    """

    def __init__(
        self,
        model_name: str,
        adapter_name: str | None,
        device: str = "cpu",
    ):
        self.model_name = model_name
        self.adapter_name = adapter_name
        self.device = device
        self.tokenizer = None
        self.model = None
        self.id2label: dict[int, str] = {}

    def load(self) -> None:
        from peft import PeftModel
        from transformers import AutoModelForTokenClassification, AutoTokenizer

        logger.info("loading tokenizer: %s", self.model_name)
        self.tokenizer = AutoTokenizer.from_pretrained(self.model_name)

        logger.info("loading base model: %s", self.model_name)
        base = AutoModelForTokenClassification.from_pretrained(self.model_name)

        if self.adapter_name:
            logger.info("loading LoRA adapter: %s", self.adapter_name)
            self.model = PeftModel.from_pretrained(base, self.adapter_name)
        else:
            self.model = base

        self.model.to(self.device)
        self.model.eval()

        # PEFT wraps the base; reach through for id2label.
        config = getattr(self.model, "config", None) or self.model.base_model.config
        self.id2label = {int(k): v for k, v in config.id2label.items()}
        logger.info("model loaded — labels: %s", sorted(set(self.id2label.values())))

    def warmup(self) -> None:
        """Run one throwaway inference so the first real request isn't a cold start."""
        if self.model is None:
            raise RuntimeError("warmup called before load")
        logger.info("running warmup inference")
        _ = self.predict("Sample text for warmup.")
        logger.info("warmup complete")

    def predict(self, text: str) -> list[RawSpan]:
        return self._predict_batch([text])[0]

    def predict_batch(self, texts: list[str]) -> list[list[RawSpan]]:
        if not texts:
            return []
        return self._predict_batch(texts)

    def _predict_batch(self, texts: list[str]) -> list[list[RawSpan]]:
        import torch

        if self.tokenizer is None or self.model is None:
            raise RuntimeError("model not loaded")

        # Empty strings can't be tokenized cleanly; short-circuit.
        all_empty = all(t == "" for t in texts)
        if all_empty:
            return [[] for _ in texts]

        encoded = self.tokenizer(
            texts,
            return_tensors="pt",
            return_offsets_mapping=True,
            truncation=True,
            padding=True,
        )
        offset_mapping = encoded.pop("offset_mapping")
        attention_mask = encoded["attention_mask"]
        encoded = {k: v.to(self.device) for k, v in encoded.items()}

        with torch.no_grad():
            outputs = self.model(**encoded)

        # logits: (batch, seq, num_labels)
        probs = torch.softmax(outputs.logits, dim=-1)
        scores, label_ids = probs.max(dim=-1)

        scores = scores.cpu()
        label_ids = label_ids.cpu()

        results: list[list[RawSpan]] = []
        for i, text in enumerate(texts):
            spans = self._extract_spans(
                text=text,
                offsets=offset_mapping[i].tolist(),
                attention=attention_mask[i].tolist(),
                label_ids=label_ids[i].tolist(),
                scores=scores[i].tolist(),
            )
            spans = self._postprocess(text, spans)
            results.append(spans)
        return results

    def _extract_spans(
        self,
        text: str,
        offsets: list[list[int]],
        attention: list[int],
        label_ids: list[int],
        scores: list[float],
    ) -> list[RawSpan]:
        """Walk tokens left-to-right, group consecutive same-label tokens, return char spans."""
        char_to_byte = _char_to_byte_offsets(text)
        spans: list[RawSpan] = []
        active_label: str | None = None
        active_start: int = 0
        active_end: int = 0
        active_score_sum: float = 0.0
        active_score_count: int = 0

        def close():
            nonlocal active_label, active_score_sum, active_score_count
            if active_label is None:
                return
            byte_start = char_to_byte[active_start]
            byte_end = char_to_byte[active_end]
            spans.append(
                RawSpan(
                    label=active_label,
                    text=text[active_start:active_end],
                    start=byte_start,
                    end=byte_end,
                    score=active_score_sum / max(active_score_count, 1),
                )
            )
            active_label = None
            active_score_sum = 0.0
            active_score_count = 0

        for tok_idx, ((char_start, char_end), mask, label_id, score) in enumerate(
            zip(offsets, attention, label_ids, scores)
        ):
            if mask == 0:
                continue
            # Special tokens (CLS, SEP, PAD) have offset (0,0).
            if char_start == 0 and char_end == 0 and tok_idx != 0:
                close()
                continue
            label = self.id2label.get(int(label_id), "O")
            label = _strip_bio(label)
            if label == "O" or label == "":
                close()
                continue

            if active_label is None:
                active_label = label
                active_start = char_start
                active_end = char_end
                active_score_sum = score
                active_score_count = 1
            elif label == active_label:
                active_end = char_end
                active_score_sum += score
                active_score_count += 1
            else:
                close()
                active_label = label
                active_start = char_start
                active_end = char_end
                active_score_sum = score
                active_score_count = 1

        close()
        return spans

    def _postprocess(self, text: str, spans: list[RawSpan]) -> list[RawSpan]:
        """Trim whitespace at span edges; merge adjacent same-label spans.

        The upstream `naija-privacy-filter` README mentions deterministic span
        postprocessing for boundary issues. We implement a minimal version:
        trim whitespace at span edges and merge adjacent spans of the same
        label separated only by whitespace.

        Computes the char→byte and bytes-of-text caches once per call rather
        than per span — relevant for long inputs with many candidate spans.
        """
        if not spans:
            return spans

        char_to_byte = _char_to_byte_offsets(text)
        text_bytes = text.encode("utf-8")

        cleaned: list[RawSpan] = []
        for span in spans:
            # Source whitespace counts from the actual underlying text rather
            # than span.text — the tokenizer can return decoded text that
            # doesn't byte-exactly match text[start:end] (subword joining,
            # special chars, etc.). Trusting span.text led to off-by-N drift.
            char_start = _byte_to_char(text, span.start)
            char_end = _byte_to_char(text, span.end)
            actual = text[char_start:char_end]
            stripped_text = actual.strip()
            if not stripped_text:
                continue
            leading = len(actual) - len(actual.lstrip())
            trailing = len(actual) - len(actual.rstrip())
            if leading or trailing:
                new_char_start = char_start + leading
                new_char_end = char_end - trailing
                if new_char_end <= new_char_start:
                    continue
                span = RawSpan(
                    label=span.label,
                    text=stripped_text,
                    start=char_to_byte[new_char_start],
                    end=char_to_byte[new_char_end],
                    score=span.score,
                )
            cleaned.append(span)

        merged: list[RawSpan] = []
        for span in cleaned:
            if merged and merged[-1].label == span.label:
                between = text_bytes[merged[-1].end:span.start].decode("utf-8", errors="ignore")
                if between.strip() == "":
                    prev = merged[-1]
                    merged[-1] = RawSpan(
                        label=prev.label,
                        text=text_bytes[prev.start:span.end].decode("utf-8", errors="ignore"),
                        start=prev.start,
                        end=span.end,
                        score=(prev.score + span.score) / 2,
                    )
                    continue
            merged.append(span)
        return merged


def _char_to_byte_offsets(text: str) -> list[int]:
    offsets = [0]
    running = 0
    for ch in text:
        running += len(ch.encode("utf-8"))
        offsets.append(running)
    return offsets


def _byte_to_char(text: str, byte_offset: int) -> int:
    if byte_offset <= 0:
        return 0
    running = 0
    for i, ch in enumerate(text):
        if running >= byte_offset:
            return i
        running += len(ch.encode("utf-8"))
    return len(text)


def _strip_bio(label: str) -> str:
    if label.startswith(("B-", "I-")):
        return label[2:]
    return label


def build_engine_from_env() -> InferenceEngine:
    model_name = os.environ.get("PRIVACY_FILTER_MODEL_NAME", "openai/privacy-filter")
    adapter_name = os.environ.get("PRIVACY_FILTER_ADAPTER_NAME") or None
    device = os.environ.get("PRIVACY_FILTER_DEVICE", "cpu")
    if device == "cuda":
        import torch

        if not torch.cuda.is_available():
            logger.warning("PRIVACY_FILTER_DEVICE=cuda but CUDA unavailable; falling back to cpu")
            device = "cpu"
    return InferenceEngine(model_name=model_name, adapter_name=adapter_name, device=device)
