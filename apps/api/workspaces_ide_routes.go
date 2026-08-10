package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

const workspaceOutputLimit = 128 * 1024
const workspaceFileWriteScript = `set -eu
p=$1
expected=$2
[ -f "$p" ] && [ ! -L "$p" ] || exit 42
tmp=$(mktemp "$p.vivarium-new.XXXXXX")
backup=$(mktemp "$p.vivarium-old.XXXXXX")
rm -f "$backup"
trap 'rm -f "$tmp" "$backup"' EXIT
cat >"$tmp"
mv "$p" "$backup"
actual=$(sha256sum "$backup" | cut -d' ' -f1)
if [ "$actual" != "$expected" ]; then
	[ -e "$p" ] || mv "$backup" "$p"
	exit 41
fi
if ! ln "$tmp" "$p"; then
	[ -e "$p" ] || mv "$backup" "$p"
	exit 41
fi
actual=$(sha256sum "$backup" | cut -d' ' -f1)
if [ "$actual" != "$expected" ]; then
	[ ! "$p" -ef "$tmp" ] || rm -f "$p"
	[ -e "$p" ] || mv "$backup" "$p"
	exit 41
fi
rm -f "$tmp" "$backup"
trap - EXIT`

func registerWorkspaceIDERoutes(mux *http.ServeMux, catalog *repositories.Store, store *workspaces.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /workspaces/{workspace_id}/files", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		dir, ok := workspacePath(w, r.URL.Query().Get("path"))
		if !ok {
			return
		}
		out, err := workspaceAuthorizedExec(catalog, item, actor, false, 15*time.Second, dir, nil, "find", ".", "-mindepth", "1", "-maxdepth", "1", "-printf", "%y\t%s\t%f\n")
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		type entry struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
			Size int    `json:"size"`
		}
		entries := []entry{}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			p := strings.SplitN(line, "\t", 3)
			if len(p) != 3 {
				continue
			}
			size, _ := strconv.Atoi(p[1])
			kind := "file"
			if p[0] == "d" {
				kind = "directory"
			}
			relative := strings.TrimPrefix(path.Join(dir, p[2]), "/workspace/")
			entries = append(entries, entry{p[2], relative, kind, size})
		}
		writeJSON(w, 200, map[string]any{"commit_id": item.CommitID, "path": strings.TrimPrefix(dir, "/workspace"), "entries": entries})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/file", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		name, ok := workspaceFilePath(w, r.URL.Query().Get("path"))
		if !ok {
			return
		}
		out, err := workspaceAuthorizedExec(catalog, item, actor, false, 10*time.Second, path.Dir(name), nil, "sh", "-c", "exec 3<\"$1\"; resolved=$(readlink -f /proc/self/fd/3); case \"$resolved\" in /workspace/*) cat <&3 ;; *) exit 42 ;; esac", "sh", path.Base(name))
		if err != nil {
			writeAPIError(w, 404, "workspace_file_not_found", "workspace file not found")
			return
		}
		if len(out) > workspaceOutputLimit {
			writeAPIError(w, 422, "workspace_file_too_large", "editable files are limited to 128 KiB")
			return
		}
		sum := sha256.Sum256(out)
		writeJSON(w, 200, map[string]any{"path": strings.TrimPrefix(name, "/workspace/"), "content": string(out), "sha256": hex.EncodeToString(sum[:]), "commit_id": item.CommitID})
	})
	mux.HandleFunc("PUT /workspaces/{workspace_id}/file", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Path           string `json:"path"`
			Content        string `json:"content"`
			ExpectedSHA256 string `json:"expected_sha256"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if len(input.Content) > workspaceOutputLimit {
			writeAPIError(w, 422, "workspace_file_too_large", "editable files are limited to 128 KiB")
			return
		}
		if !validWorkspaceDigest(input.ExpectedSHA256) {
			writeAPIError(w, 422, "workspace_file_digest_invalid", "expected_sha256 must be a complete SHA-256 digest")
			return
		}
		name, valid := workspaceFilePath(w, input.Path)
		if !valid {
			return
		}
		err := workspaceControlledExec(store, catalog, item, actor, "files", 10*time.Second, path.Dir(name), strings.NewReader(input.Content), "sh", "-c", workspaceFileWriteScript, "sh", path.Base(name), input.ExpectedSHA256)
		if err != nil {
			if errors.Is(err, workspaces.ErrControl) {
				writeAPIError(w, 409, "workspace_control_required", "live file control is held by another participant or has expired")
				return
			}
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 41 {
				writeAPIError(w, 409, "workspace_file_changed", "file changed since it was opened")
				return
			}
			writeRuntimeError(w, err)
			return
		}
		sum := sha256.Sum256([]byte(input.Content))
		change := workspaces.Change{Path: input.Path, SHA256: hex.EncodeToString(sum[:]), Size: len(input.Content), ActorID: actor.UserID, CreatedAt: time.Now().UTC()}
		updated, err := store.RecordChange(item.ID, change)
		if err != nil {
			writeAPIError(w, 500, "workspace_change_failed", "file changed but evidence could not be saved")
			return
		}
		writeJSON(w, 200, map[string]any{"change": change, "workspace": updated})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/search", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" || len(q) > 200 {
			writeAPIError(w, 422, "workspace_search_invalid", "q must be 1-200 characters")
			return
		}
		out, err := workspaceAuthorizedExec(catalog, item, actor, false, 15*time.Second, "/workspace", nil, "grep", "-RInF", "--exclude-dir=.git", "--", q, ".")
		if err != nil {
			var x *exec.ExitError
			if !errors.As(err, &x) || x.ExitCode() != 1 {
				writeRuntimeError(w, err)
				return
			}
		}
		if len(out) > workspaceOutputLimit {
			out = out[:workspaceOutputLimit]
		}
		writeJSON(w, 200, map[string]any{"query": q, "matches": strings.Split(strings.TrimSpace(string(out)), "\n"), "commit_id": item.CommitID})
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/commands", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Command        string `json:"command"`
			Directory      string `json:"directory"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if strings.TrimSpace(input.Command) == "" || len(input.Command) > 4000 {
			writeAPIError(w, 422, "workspace_command_invalid", "command must be 1-4000 characters")
			return
		}
		if input.TimeoutSeconds == 0 {
			input.TimeoutSeconds = 60
		}
		if input.TimeoutSeconds < 1 || input.TimeoutSeconds > 300 {
			writeAPIError(w, 422, "workspace_command_invalid", "timeout_seconds must be 1-300")
			return
		}
		dir, valid := workspacePath(w, input.Directory)
		if !valid {
			return
		}
		start := time.Now().UTC()
		var out []byte
		runErr := store.WithControl(item.ID, actor.UserID, "commands", func(current workspaces.Workspace) error {
			var executeErr error
			out, executeErr = workspaceAuthorizedExec(catalog, current, actor, true, time.Duration(input.TimeoutSeconds)*time.Second, dir, nil, "sh", "-lc", input.Command)
			return executeErr
		})
		if errors.Is(runErr, workspaces.ErrControl) {
			writeAPIError(w, 409, "workspace_control_required", "live command control is held by another participant or has expired")
			return
		}
		code := 0
		if runErr != nil {
			code = -1
			var x *exec.ExitError
			if errors.As(runErr, &x) {
				code = x.ExitCode()
			}
		}
		if len(out) > workspaceOutputLimit {
			out = append(out[:workspaceOutputLimit], []byte("\n[output truncated]")...)
		}
		commandDigest := sha256.Sum256([]byte(input.Command))
		outcome := workspaces.CommandOutcome{CommandSHA256: hex.EncodeToString(commandDigest[:]), Directory: strings.TrimPrefix(dir, "/workspace"), ExitCode: code, Output: string(out), ActorID: actor.UserID, StartedAt: start, CompletedAt: time.Now().UTC()}
		updated, err := store.RecordCommand(item.ID, outcome)
		if err != nil {
			writeAPIError(w, 500, "workspace_command_evidence_failed", "command finished but evidence could not be saved")
			return
		}
		writeJSON(w, 200, map[string]any{"outcome": updated.Commands[len(updated.Commands)-1], "commit_id": item.CommitID})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/ports", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		out, err := workspaceAuthorizedExec(catalog, item, actor, false, 5*time.Second, "/workspace", nil, "sh", "-c", "cat /proc/net/tcp /proc/net/tcp6 2>/dev/null | awk 'NR>1 {split($2,a,\":\"); print a[2]}' | sort -u")
		if err != nil {
			writeRuntimeError(w, err)
			return
		}
		ports := []int{}
		for _, v := range strings.Fields(string(out)) {
			n, e := strconv.ParseInt(v, 16, 32)
			if e == nil && n > 0 {
				ports = append(ports, int(n))
			}
		}
		writeJSON(w, 200, map[string]any{"ports": ports, "preview_note": "Previews are authenticated and proxied without publishing container ports."})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}/preview", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		port, err := strconv.Atoi(r.URL.Query().Get("port"))
		if err != nil || port < 1 || port > 65535 {
			writeAPIError(w, 422, "workspace_preview_invalid", "port must be 1-65535")
			return
		}
		previewPath := r.URL.Query().Get("path")
		if previewPath == "" {
			previewPath = "/"
		}
		if !strings.HasPrefix(previewPath, "/") || strings.ContainsAny(previewPath, "\r\n") {
			writeAPIError(w, 422, "workspace_preview_invalid", "preview path must begin with /")
			return
		}
		url := fmt.Sprintf("http://127.0.0.1:%d%s", port, previewPath)
		out, runErr := workspaceAuthorizedExec(catalog, item, actor, false, 15*time.Second, "/workspace", nil, "sh", "-c", "if command -v wget >/dev/null; then wget -qO- -- \"$1\"; elif command -v curl >/dev/null; then curl -fsS --max-time 10 -- \"$1\"; else exit 127; fi", "sh", url)
		if runErr != nil {
			writeAPIError(w, 502, "workspace_preview_unavailable", "application preview did not respond")
			return
		}
		if len(out) > 1024*1024 {
			out = out[:1024*1024]
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'; img-src data:")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(200)
		_, _ = w.Write(out)
	})
}

func workspaceControlledExec(store *workspaces.Store, catalog *repositories.Store, item workspaces.Workspace, actor auth.Credential, scope string, timeout time.Duration, dir string, stdin *strings.Reader, args ...string) error {
	return store.WithControl(item.ID, actor.UserID, scope, func(current workspaces.Workspace) error {
		_, err := workspaceAuthorizedExec(catalog, current, actor, true, timeout, dir, stdin, args...)
		return err
	})
}

func authorizeRunningWorkspace(w http.ResponseWriter, r *http.Request, s *workspaces.Store, c *repositories.Store, a *auth.Store, scope string) (workspaces.Workspace, auth.Credential, bool) {
	item, actor, ok := authorizeWorkspace(w, r, s, c, a, scope)
	if ok && item.State != "running" {
		writeAPIError(w, 409, "workspace_not_running", "workspace resources require a running environment")
		return item, actor, false
	}
	return item, actor, ok
}
func workspacePath(w http.ResponseWriter, value string) (string, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
	clean := path.Clean("/" + value)
	if clean == "/." {
		clean = "/"
	}
	if strings.Contains(value, "\x00") || strings.HasPrefix(clean, "/../") {
		writeAPIError(w, 422, "workspace_path_invalid", "path must stay inside the workspace")
		return "", false
	}
	return path.Join("/workspace", clean), true
}
func workspaceFilePath(w http.ResponseWriter, value string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		writeAPIError(w, 422, "workspace_path_invalid", "a file path is required")
		return "", false
	}
	return workspacePath(w, value)
}
func validWorkspaceDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
func workspaceAuthorizedExec(catalog *repositories.Store, item workspaces.Workspace, actor auth.Credential, mutation bool, timeout time.Duration, dir string, stdin *strings.Reader, args ...string) ([]byte, error) {
	var output []byte
	operation := func() error {
		wrapper := `set -eu
cd -P -- "$1"
physical=$(pwd -P)
case "$physical" in /workspace|/workspace/*) ;; *) exit 42 ;; esac
shift
exec "$@"`
		wrapped := []string{"sh", "-c", wrapper, "sh", dir}
		wrapped = append(wrapped, args...)
		var err error
		output, err = workspaceExec(item.ID, timeout, "/workspace", stdin, wrapped...)
		return err
	}
	var err error
	if mutation {
		err = catalog.WithCurrentParticipant(actor.UserID, item.RepositoryID, operation)
	} else {
		err = catalog.WithCurrentReadAccess(actor.UserID, []string{item.RepositoryID}, operation)
	}
	return output, err
}
func workspaceExec(id string, timeout time.Duration, dir string, stdin *strings.Reader, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	dockerArgs := []string{"exec", "--workdir", dir, "vivarium-workspace-" + id}
	dockerArgs = append(dockerArgs, args...)
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return out, fmt.Errorf("workspace command timed out")
	}
	return out, err
}
func writeRuntimeError(w http.ResponseWriter, err error) {
	if errors.Is(err, repositories.ErrNotFound) || errors.Is(err, repositories.ErrInvalidCollaborator) {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return
	}
	writeAPIError(w, 422, "workspace_runtime_failed", strings.TrimSpace(err.Error()))
}
