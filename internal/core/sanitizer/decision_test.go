package sanitizer

import (
	"context"
	"testing"
)

func TestRequestMode_Unset(t *testing.T) {
	mode, set := RequestMode(context.Background())
	if set {
		t.Fatalf("expected set=false on bare context, got mode=%q", mode)
	}
	if mode != "" {
		t.Fatalf("expected empty mode on bare context, got %q", mode)
	}
}

func TestWithRequestMode_EmptyIsNoop(t *testing.T) {
	parent := context.Background()
	child := WithRequestMode(parent, "")
	if child != parent {
		t.Fatalf("expected empty mode to return parent unchanged")
	}
	if _, set := RequestMode(child); set {
		t.Fatalf("expected no override after WithRequestMode(ctx, \"\")")
	}
}

func TestRequestMode_RoundTrip(t *testing.T) {
	for _, want := range []string{"redact", "tokenize"} {
		ctx := WithRequestMode(context.Background(), want)
		got, set := RequestMode(ctx)
		if !set {
			t.Fatalf("%q: expected set=true", want)
		}
		if got != want {
			t.Fatalf("expected mode=%q, got %q", want, got)
		}
	}
}
