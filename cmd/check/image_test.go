package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedImageCleanupChecksOwnershipAndReferences(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	tag := "chattui-check-shared:owned"
	id := "sha256:" + strings.Repeat("a", 64)
	script := "#!/bin/sh\nif [ \"$1\" = image ] && [ \"$2\" = inspect ]; then printf '%s\\n' '[{\"Id\":\"" + id + "\",\"RepoTags\":[\"" + tag + "\"],\"Config\":{\"Labels\":{\"chattui.check.image-owner\":\"foreign\"}}}]'; exit 0; fi\nif [ \"$1\" = ps ]; then exit 0; fi\nprintf '%s\\n' \"$*\" >> '" + log + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := removeSharedImage(imageRecord{ID: id, Tag: tag, Owner: "owned"}); err == nil {
		t.Fatal("deleted foreign image")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("image rm called: %v", err)
	}
	script = strings.Replace(script, "foreign", "owned", 1)
	busy := strings.Replace(script, "if [ \"$1\" = ps ]; then exit 0; fi", "if [ \"$1\" = ps ]; then echo foreign-container; exit 0; fi", 1)
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(busy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeSharedImage(imageRecord{ID: id, Tag: tag, Owner: "owned"}); err == nil {
		t.Fatal("deleted image referenced by container")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("image rm called: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeSharedImage(imageRecord{ID: id, Tag: tag, Owner: "owned"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log)
	if err != nil || strings.TrimSpace(string(data)) != "image rm "+tag {
		t.Fatalf("wrong cleanup: %q %v", data, err)
	}
}
