package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSplitText(t *testing.T) {
	got := splitText("Hello world. This is a test.", 15)
	if len(got) < 2 {
		t.Fatalf("expected >=2 chunks, got %v", got)
	}
}

func TestExtractMarkdown(t *testing.T) {
	got := extractText("# Hi\n\n**bold** and `code` and [link](http://x)", ".md")
	if got == "" {
		t.Fatal("empty")
	}
}

func TestGoogleEndToEnd(t *testing.T) {
	if os.Getenv("LTTS_ONLINE_TEST") == "" {
		t.Skip("set LTTS_ONLINE_TEST=1 to run")
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.mp3")
	eng := &googleEngine{}
	voice := eng.Voices()[0]
	err := Synthesize(context.Background(), eng, "Hello from the test.", voice, false, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(out)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("bad output: %v size=%d", err, fi.Size())
	}
}
