package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCursorKeyRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cursor-key")
	if err := os.WriteFile(path, bytes.Repeat([]byte{9}, 32), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := cursorCodec(path)
	if err != nil {
		t.Fatal(err)
	}
	token, err := first.Encode("scope", "position", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := cursorCodec(path)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err = restarted.Decode(token, "scope", &got); err != nil || got != "position" {
		t.Fatalf("restart: %q %v", got, err)
	}
	ephemeral, err := cursorCodec("")
	if err != nil {
		t.Fatal(err)
	}
	if err = ephemeral.Decode(token, "scope", &got); err == nil {
		t.Fatal("fresh key accepted old cursor")
	}
	if err = os.WriteFile(path, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = cursorCodec(path); err == nil {
		t.Fatal("short key accepted")
	}
}
