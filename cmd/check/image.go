package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const imageOwnerLabel = "chattui.check.image-owner"

type imageRecord struct {
	ID    string `json:"id"`
	Tag   string `json:"tag"`
	Owner string `json:"owner"`
}

func dockerCommand(ctx context.Context, args ...string) (string, error) {
	c := exec.CommandContext(ctx, "docker", args...)
	out, e := c.Output()
	if e != nil {
		return "", fmt.Errorf("docker %s: %w", strings.Join(args[:min(2, len(args))], " "), e)
	}
	return strings.TrimSpace(string(out)), nil
}
func buildSharedImage(ctx context.Context, repo, dir string) (imageRecord, error) {
	var r imageRecord
	raw := make([]byte, 12)
	if _, e := rand.Read(raw); e != nil {
		return r, e
	}
	r.Owner = hex.EncodeToString(raw)
	r.Tag = "chattui-check-shared:" + r.Owner
	c := exec.CommandContext(ctx, "docker", "build", "--label", imageOwnerLabel+"="+r.Owner, "-t", r.Tag, repo)
	out, e := c.CombinedOutput()
	if writeErr := os.WriteFile(filepath.Join(dir, "server-build.log"), out, 0600); writeErr != nil {
		return r, writeErr
	}
	if e != nil {
		return r, fmt.Errorf("server image build failed: %w; inspect server-build.log", e)
	}
	// Keep a cleanup handle even if inspect fails after build.
	inspect, e := inspectImage(ctx, r.Tag)
	if e != nil {
		return r, errors.Join(e, removeSharedImage(r))
	}
	if inspect.Owner != r.Owner || len(inspect.Tags) != 1 || inspect.Tags[0] != r.Tag {
		return r, errors.Join(errors.New("built image ownership or tags mismatch"), removeSharedImage(r))
	}
	r.ID = inspect.ID
	if e = writeJSON(filepath.Join(dir, "shared-image.json"), r); e != nil {
		return r, errors.Join(e, removeSharedImage(r))
	}
	return r, nil
}

type imageInspect struct {
	ID     string   `json:"Id"`
	Tags   []string `json:"RepoTags"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	Owner string `json:"-"`
}

func inspectImage(ctx context.Context, tag string) (imageInspect, error) {
	var r imageInspect
	out, e := dockerCommand(ctx, "image", "inspect", tag)
	if e != nil {
		return r, e
	}
	var imgs []imageInspect
	if e = json.Unmarshal([]byte(out), &imgs); e != nil || len(imgs) != 1 {
		return r, errors.New("invalid image inspect response")
	}
	r = imgs[0]
	r.Owner = r.Config.Labels[imageOwnerLabel]
	return r, nil
}
func removeSharedImage(r imageRecord) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	inspect, e := inspectImage(ctx, r.Tag)
	if e != nil {
		return e
	}
	if inspect.Owner != r.Owner || len(inspect.Tags) != 1 || inspect.Tags[0] != r.Tag || (r.ID != "" && inspect.ID != r.ID) {
		return errors.New("shared image ownership, ID or references changed; refusing deletion")
	}
	containers, e := dockerCommand(ctx, "ps", "-a", "--filter", "ancestor="+inspect.ID, "--format", "{{.ID}}")
	if e != nil {
		return e
	}
	if containers != "" {
		return errors.New("shared image still referenced by containers; refusing deletion")
	}
	_, e = dockerCommand(ctx, "image", "rm", r.Tag)
	return e
}
func verifyServerImage(ctx context.Context, serverID, pinned, dir string) error {
	id, e := dockerCommand(ctx, "inspect", "--format", "{{.Image}}", serverID)
	if e != nil {
		return e
	}
	if e = writeJSON(filepath.Join(dir, "server-image.json"), struct {
		ID       string `json:"id"`
		PinnedID string `json:"pinned_id"`
	}{id, pinned}); e != nil {
		return e
	}
	if id != pinned {
		return fmt.Errorf("server image ID differs from pinned shared image")
	}
	return nil
}
