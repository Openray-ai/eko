"""Tests for the sidecar inference path.

Loading the real model is expensive, so the integration test that actually
exercises the model is gated behind RUN_SLM_INTEGRATION=1. The unit tests
here exercise the byte-offset helpers and the span-extraction / post-processing
logic without touching torch — which is exactly the code most likely to
regress when offsets, BIO tagging, or whitespace handling change.
"""

from __future__ import annotations

import os

import pytest

from app.inference import (
    InferenceEngine,
    RawSpan,
    _byte_to_char,
    _char_to_byte_offsets,
    _strip_bio,
)


# ---------------------------------------------------------------------------
# Byte-offset helpers
# ---------------------------------------------------------------------------


def test_char_to_byte_offsets_ascii():
    assert _char_to_byte_offsets("hello") == [0, 1, 2, 3, 4, 5]


def test_char_to_byte_offsets_unicode():
    # ù is two bytes in UTF-8.
    text = "Yùsuf"
    offsets = _char_to_byte_offsets(text)
    assert offsets == [0, 1, 3, 4, 5, 6]


def test_byte_to_char_roundtrip():
    text = "Amina Yùsuf"
    offsets = _char_to_byte_offsets(text)
    for char_idx, byte_idx in enumerate(offsets):
        assert _byte_to_char(text, byte_idx) == char_idx


def test_strip_bio():
    assert _strip_bio("B-private_phone") == "private_phone"
    assert _strip_bio("I-private_phone") == "private_phone"
    assert _strip_bio("private_phone") == "private_phone"
    assert _strip_bio("O") == "O"


# ---------------------------------------------------------------------------
# _extract_spans — fake tokenizer output, no torch
# ---------------------------------------------------------------------------


def _make_engine(id2label: dict[int, str]) -> InferenceEngine:
    """Build an engine instance without calling load(); set id2label directly."""
    eng = InferenceEngine(model_name="fake", adapter_name=None, device="cpu")
    eng.id2label = id2label
    return eng


def test_extract_spans_basic():
    """Two consecutive person tokens fold into one span across the whitespace."""
    eng = _make_engine({0: "O", 1: "private_person"})
    text = "Amina Yusuf called."
    # Pretend the tokenizer split into [CLS, "Amina", " Yusuf", " called", ".", SEP]
    offsets = [(0, 0), (0, 5), (5, 11), (11, 18), (18, 19), (0, 0)]
    attention = [1, 1, 1, 1, 1, 1]
    label_ids = [0, 1, 1, 0, 0, 0]
    scores = [0.9, 0.95, 0.92, 0.9, 0.9, 0.9]

    spans = eng._extract_spans(text, [list(o) for o in offsets], attention, label_ids, scores)
    assert len(spans) == 1
    s = spans[0]
    assert s.label == "private_person"
    assert s.start == 0  # byte offset
    assert s.end == 11
    assert text[s.start : s.end] == "Amina Yusuf"


def test_extract_spans_handles_bio_prefixes():
    eng = _make_engine({0: "O", 1: "B-private_phone", 2: "I-private_phone"})
    text = "+234 802"
    offsets = [(0, 0), (0, 4), (4, 8), (0, 0)]
    attention = [1, 1, 1, 1]
    label_ids = [0, 1, 2, 0]
    scores = [0.9, 0.95, 0.93, 0.9]

    spans = eng._extract_spans(text, [list(o) for o in offsets], attention, label_ids, scores)
    assert len(spans) == 1
    assert spans[0].label == "private_phone"
    assert text[spans[0].start : spans[0].end] == "+234 802"


def test_extract_spans_separates_different_labels():
    eng = _make_engine({0: "O", 1: "private_person", 2: "private_phone"})
    text = "Amina +234"
    offsets = [(0, 0), (0, 5), (6, 10), (0, 0)]
    attention = [1, 1, 1, 1]
    label_ids = [0, 1, 2, 0]
    scores = [0.9, 0.9, 0.9, 0.9]

    spans = eng._extract_spans(text, [list(o) for o in offsets], attention, label_ids, scores)
    assert len(spans) == 2
    labels = [s.label for s in spans]
    assert labels == ["private_person", "private_phone"]


def test_extract_spans_byte_offsets_for_unicode():
    """Byte offsets must match UTF-8 byte positions for non-ASCII text."""
    eng = _make_engine({0: "O", 1: "private_person"})
    text = "Yùsuf"  # 'ù' is two UTF-8 bytes
    # Pretend tokenizer kept it as one piece.
    offsets = [(0, 0), (0, 5), (0, 0)]
    attention = [1, 1, 1]
    label_ids = [0, 1, 0]
    scores = [0.9, 0.95, 0.9]

    spans = eng._extract_spans(text, [list(o) for o in offsets], attention, label_ids, scores)
    assert len(spans) == 1
    # 5 chars, but 6 UTF-8 bytes
    assert spans[0].start == 0
    assert spans[0].end == 6


def test_extract_spans_skips_padding_tokens():
    eng = _make_engine({0: "O", 1: "private_person"})
    text = "Hi"
    offsets = [(0, 0), (0, 2), (0, 0), (0, 0)]
    attention = [1, 1, 0, 0]  # last two are padding
    label_ids = [0, 1, 1, 1]  # would be a hit if not masked
    scores = [0.9, 0.9, 0.9, 0.9]

    spans = eng._extract_spans(text, [list(o) for o in offsets], attention, label_ids, scores)
    assert len(spans) == 1
    assert spans[0].label == "private_person"
    assert spans[0].end == 2


# ---------------------------------------------------------------------------
# _postprocess
# ---------------------------------------------------------------------------


def test_postprocess_trims_whitespace():
    eng = _make_engine({})
    text = "Hello   Amina Yusuf  end"
    # Suppose model emitted a span that included surrounding spaces.
    raw = [RawSpan(label="private_person", text="  Amina Yusuf  ", start=5, end=20, score=0.9)]
    out = eng._postprocess(text, raw)
    assert len(out) == 1
    assert out[0].text == "Amina Yusuf"
    # Adjusted byte offsets should land on the actual name, not whitespace.
    assert text[out[0].start : out[0].end] == "Amina Yusuf"


def test_postprocess_drops_whitespace_only_spans():
    eng = _make_engine({})
    text = "foo   bar"
    # Span covers the run of spaces between "foo" and "bar" — text[3:6] == "   ".
    raw = [RawSpan(label="private_person", text="   ", start=3, end=6, score=0.5)]
    assert eng._postprocess(text, raw) == []


def test_postprocess_disambiguates_repeated_text():
    """The deterministic offset adjustment must NOT relocate the span when the
    stripped text appears more than once. (Older code used text.find which
    could pick the wrong occurrence.)"""
    eng = _make_engine({})
    text = "Amina Amina Amina"  # three identical names
    # Span originally covers the SECOND "Amina" (chars 6..11, also bytes 6..11)
    # but the model emitted it with a leading space included.
    raw = [RawSpan(label="private_person", text=" Amina", start=5, end=11, score=0.9)]
    out = eng._postprocess(text, raw)
    assert len(out) == 1
    assert out[0].start == 6
    assert out[0].end == 11
    assert text[out[0].start : out[0].end] == "Amina"


def test_postprocess_merges_adjacent_same_label():
    eng = _make_engine({})
    text = "Amina  Yusuf"
    raw = [
        RawSpan(label="private_person", text="Amina", start=0, end=5, score=0.9),
        RawSpan(label="private_person", text="Yusuf", start=7, end=12, score=0.8),
    ]
    out = eng._postprocess(text, raw)
    assert len(out) == 1
    assert out[0].text == "Amina  Yusuf"
    assert out[0].start == 0
    assert out[0].end == 12


def test_postprocess_does_not_merge_different_labels():
    eng = _make_engine({})
    text = "Amina +234"
    raw = [
        RawSpan(label="private_person", text="Amina", start=0, end=5, score=0.9),
        RawSpan(label="private_phone", text="+234", start=6, end=10, score=0.9),
    ]
    out = eng._postprocess(text, raw)
    assert len(out) == 2


# ---------------------------------------------------------------------------
# Integration — only when explicitly requested
# ---------------------------------------------------------------------------


@pytest.mark.skipif(
    os.environ.get("RUN_SLM_INTEGRATION") != "1",
    reason="set RUN_SLM_INTEGRATION=1 to run model integration tests",
)
def test_predict_real_model():
    from app.inference import build_engine_from_env

    engine = build_engine_from_env()
    engine.load()
    engine.warmup()
    spans = engine.predict("Amina Yusuf can be reached at +234 802 111 3344.")
    labels = {s.label for s in spans}
    assert "private_phone" in labels or "private_person" in labels
    for span in spans:
        assert span.start < span.end
        assert span.score > 0
