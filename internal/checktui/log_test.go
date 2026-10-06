package checktui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveClientLogKeepsOnlyKnownEventNames(t *testing.T) {
	client := t.TempDir()
	results := t.TempDir()
	source := "time=now level=INFO msg=authenticated username=example token=secret-token\n" +
		"time=now level=INFO msg=\"websocket connected\" url=ws://127.0.0.1:1/api/v1/ws\n" +
		"time=now level=ERROR msg=unexpected password=secret-password\n"
	if e := os.WriteFile(filepath.Join(client, "client.log"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	if e := saveClientLog(results, "client-a", client); e != nil {
		t.Fatal(e)
	}
	out, e := os.ReadFile(filepath.Join(results, "client-a.log"))
	if e != nil {
		t.Fatal(e)
	}
	if string(out) != "authenticated: 1\nwebsocket connected: 1\n" {
		t.Fatalf("unexpected sanitized summary: %q", out)
	}
	for _, v := range []string{"secret-token", "secret-password", "username=", "url="} {
		if strings.Contains(string(out), v) {
			t.Fatal("raw client field persisted")
		}
	}
}
