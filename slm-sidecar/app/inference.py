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
        import torch
        from peft import PeftModel
        from transformers import AutoModelForTokenClassification, AutoTokenizer

        # The adapter's classifier head usually has a different label count
        # than the base model (iamSamurai/privacy-filter-nigeria publishes 53
        # BILOU-tagged labels vs openai/privacy-filter's 33). The upstream
        # `naija-privacy-filter` repo handles this by:
        #   1. Reading the adapter's published label_map.json
        #   2. Manually resizing the base model's score head before loading
        #      the adapter, copying weights for any labels that exist on both
        #      sides
        #   3. Re-setting id2label/label2id metadata after PEFT wraps the model
        # We follow the same approach so behaviour matches the upstream CLI.

        # Tokenizer: prefer the adapter's tokenizer files when present (the
        # adapter may have updated specials or vocab) and fall back to the
        # base. Matches naija-privacy-filter's `tokenizer_source` choice.
        tokenizer_source = self.adapter_name or self.model_name
        logger.info("loading tokenizer: %s", tokenizer_source)
        try:
            self.tokenizer = AutoTokenizer.from_pretrained(tokenizer_source)
        except Exception:  # adapter repo may not ship tokenizer files
            logger.info(
                "tokenizer not found at %s, falling back to base model %s",
                tokenizer_source,
                self.model_name,
            )
            self.tokenizer = AutoTokenizer.from_pretrained(self.model_name)

        token_label_names = (
            _load_adapter_label_map(self.adapter_name) if self.adapter_name else None
        )

        logger.info("loading base model: %s", self.model_name)
        self.model = AutoModelForTokenClassification.from_pretrained(self.model_name)

        if token_label_names is not None:
            logger.info(
                "resizing classifier head from %d → %d labels",
                self.model.config.num_labels,
                len(token_label_names),
            )
            _resize_token_classifier_head(self.model, token_label_names)

        if self.adapter_name:
            logger.info("loading LoRA adapter: %s", self.adapter_name)
            self.model = PeftModel.from_pretrained(self.model, self.adapter_name)
            if token_label_names is not None:
                # PEFT wraps the model; re-apply metadata so config.id2label
                # reflects the adapter's labels (otherwise we'd see the base's
                # smaller mapping or PeftModel's defaults).
                _set_token_label_metadata(self.model, token_label_names)

        self.model.to(self.device)
        self.model.eval()

        config = getattr(self.model, "config", None) or self.model.base_model.config
        self.id2label = {int(k): v for k, v in config.id2label.items()}
        if any(v.startswith("LABEL_") for v in self.id2label.values()):
            logger.warning(
                "adapter %r did not publish a label_map.json; falling back to "
                "placeholder labels — span detection will return generic names "
                "that the Go-side label mapping does not recognize.",
                self.adapter_name,
            )
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
        """Walk tokens left-to-right, return char spans honoring BIOES prefixes.

        Mirrors the span-extraction logic in upstream naija-privacy-filter
        (privacy_filter.py). Prefix semantics:
          - B-X / S-X → start a new span (closing any active one). S- closes
            immediately after.
          - I-X / E-X → continue an active span of the same entity, or start
            a new one if there isn't one. E- closes after appending.
          - L-X / U-X (BILOU) are handled as continuation/standalone via the
            generic path; close() emits a span on label change.
          - O → close any active span.
        """
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
            spans.append(
                RawSpan(
                    label=active_label,
                    text=text[active_start:active_end],
                    start=char_to_byte[active_start],
                    end=char_to_byte[active_end],
                    score=active_score_sum / max(active_score_count, 1),
                )
            )
            active_label = None
            active_score_sum = 0.0
            active_score_count = 0

        def open_(entity: str, start: int, end: int, sc: float):
            nonlocal active_label, active_start, active_end
            nonlocal active_score_sum, active_score_count
            active_label = entity
            active_start = start
            active_end = end
            active_score_sum = sc
            active_score_count = 1

        for tok_idx, ((char_start, char_end), mask, label_id, score) in enumerate(
            zip(offsets, attention, label_ids, scores)
        ):
            if mask == 0:
                continue
            # Special tokens (CLS, SEP, PAD) have offset (0,0).
            if char_start == 0 and char_end == 0 and tok_idx != 0:
                close()
                continue
            raw_label = self.id2label.get(int(label_id), "O")
            if raw_label == "O" or raw_label == "":
                close()
                continue

            prefix, entity = _split_label_prefix(raw_label)

            if prefix == "S" or prefix == "U":  # standalone unit
                close()
                open_(entity, char_start, char_end, score)
                close()
                continue

            if prefix == "B":
                close()
                open_(entity, char_start, char_end, score)
                continue

            if prefix in ("I", "E", "L"):
                if active_label != entity:
                    close()
                    open_(entity, char_start, char_end, score)
                else:
                    active_end = char_end
                    active_score_sum += score
                    active_score_count += 1
                if prefix in ("E", "L"):
                    close()
                continue

            # Unknown prefix: treat as B-like start.
            close()
            open_(entity, char_start, char_end, score)

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
    """Strip BIO/BIOES/BILOU prefixes (B-/I-/L-/U-/E-/S-)."""
    if len(label) > 2 and label[1] == "-" and label[0] in "BILUES":
        return label[2:]
    return label


def _split_label_prefix(label: str) -> tuple[str, str]:
    """Return (prefix, entity) for a BIOES/BILOU label, or ("B", label) if no prefix.

    Matches the upstream naija-privacy-filter convention so span extraction
    semantics align: B/S start a span, I/E continue it (E closes), U is a
    standalone unit, O has no entity (handled by caller).
    """
    if len(label) > 2 and label[1] == "-" and label[0] in "BILUES":
        return label[0], label[2:]
    return "B", label


def _load_adapter_label_map(adapter_name: str) -> list[str] | None:
    """Fetch label_map.json from the adapter repo and return the ordered label list.

    Schema (matches the upstream publishing pipeline):

        {"token_label_names": ["O", "B-private_phone", ...]}
        or
        {"id_to_label": {"0": "O", "1": "B-private_phone", ...}}
    """
    import json

    from huggingface_hub import hf_hub_download
    from huggingface_hub.utils import EntryNotFoundError

    try:
        path = hf_hub_download(repo_id=adapter_name, filename="label_map.json")
    except EntryNotFoundError:
        logger.warning(
            "adapter %s did not publish label_map.json; classifier resize and "
            "id2label recovery will be skipped.",
            adapter_name,
        )
        return None

    try:
        payload = json.loads(open(path).read())
    except (OSError, json.JSONDecodeError) as e:
        logger.warning("failed to read label_map.json: %s", e)
        return None

    token_label_names = payload.get("token_label_names")
    if isinstance(token_label_names, list) and token_label_names:
        return [str(label) for label in token_label_names]

    id_to_label = payload.get("id_to_label")
    if isinstance(id_to_label, dict) and id_to_label:
        return [
            str(label)
            for _, label in sorted(
                ((int(k), v) for k, v in id_to_label.items()), key=lambda i: i[0]
            )
        ]

    logger.warning("label_map.json did not contain token_label_names or id_to_label")
    return None


def _classifier_attribute_name(model) -> str:
    if hasattr(model, "score"):
        return "score"
    if hasattr(model, "classifier"):
        return "classifier"
    raise ValueError("token-classification model has no score/classifier head")


def _set_token_label_metadata(model, token_label_names: list[str]) -> None:
    id2label = {i: name for i, name in enumerate(token_label_names)}
    label2id = {name: i for i, name in enumerate(token_label_names)}
    model.num_labels = len(token_label_names)
    model.config.num_labels = len(token_label_names)
    model.config.id2label = id2label
    model.config.label2id = label2id


def _resize_token_classifier_head(model, token_label_names: list[str]) -> None:
    """Replace the model's classifier head so it has len(token_label_names) outputs.

    Preserves any existing weights for labels that exist in both the base and
    the adapter (matched by label name); newly added labels get fresh weights
    drawn from the model's standard initializer. After PEFT loads its
    `modules_to_save` weights on top, those overlay the score head with the
    adapter's trained values, so this function's job is purely to make the
    shapes match before that overlay happens.
    """
    import torch

    classifier_attr = _classifier_attribute_name(model)
    old_head = getattr(model, classifier_attr)
    if old_head.out_features == len(token_label_names):
        _set_token_label_metadata(model, token_label_names)
        return

    old_weight = old_head.weight.detach().cpu().clone()
    old_bias = old_head.bias.detach().cpu().clone() if old_head.bias is not None else None
    old_label_to_id = {
        str(label): int(idx)
        for idx, label in getattr(model.config, "id2label", {}).items()
    }

    new_head = torch.nn.Linear(
        old_head.in_features,
        len(token_label_names),
        bias=old_head.bias is not None,
        device=old_head.weight.device,
        dtype=old_head.weight.dtype,
    )
    std = float(getattr(model.config, "initializer_range", 0.02))
    torch.nn.init.normal_(new_head.weight, mean=0.0, std=std)
    if new_head.bias is not None:
        torch.nn.init.zeros_(new_head.bias)

    with torch.no_grad():
        for new_idx, name in enumerate(token_label_names):
            old_idx = old_label_to_id.get(name)
            if old_idx is None or old_idx >= old_weight.shape[0]:
                continue
            new_head.weight[new_idx].copy_(old_weight[old_idx])
            if new_head.bias is not None and old_bias is not None:
                new_head.bias[new_idx].copy_(old_bias[old_idx])

    setattr(model, classifier_attr, new_head)
    _set_token_label_metadata(model, token_label_names)


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
