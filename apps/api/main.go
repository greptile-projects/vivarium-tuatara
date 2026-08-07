package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

const (
	uploadPackService  = "git-upload-pack"
	receivePackService = "git-receive-pack"
	primaryBranchHook  = `#!/bin/sh
while read -r old new ref
do
	if test "$ref" != "refs/heads/main"
	then
		echo "only refs/heads/main may be updated" >&2
		exit 1
	fi
done
`
)

func main() {
	root := os.Getenv("GIT_STORAGE_ROOT")
	if root == "" {
		root = "repositories"
	}
	store, err := storage.New(root)
	if err != nil {
		log.Fatal(err)
	}
	userRoot := os.Getenv("USER_STORAGE_ROOT")
	if userRoot == "" {
		userRoot = "users"
	}
	userStore, err := users.New(userRoot)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, newAppHandler(store, userStore)); err != nil {
		log.Fatal(err)
	}
}

func newHandler(store *storage.Store) http.Handler {
	return newAppHandler(store, nil)
}

func newAppHandler(store *storage.Store, userStore *users.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	if userStore != nil {
		registerUserRoutes(mux, userStore)
	}
	mux.HandleFunc("GET /git/{remote}/info/refs", func(w http.ResponseWriter, r *http.Request) {
		service := r.URL.Query().Get("service")
		if service != uploadPackService && service != receivePackService {
			http.Error(w, "unsupported Git service", http.StatusBadRequest)
			return
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
		setGitCacheHeaders(w)
		if _, err := io.WriteString(w, pktLine("# service="+service+"\n")+"0000"); err != nil {
			return
		}
		runGitService(w, r, repo, service, true)
	})
	mux.HandleFunc("POST /git/{remote}/git-upload-pack", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, uploadPackService, false)
	})
	mux.HandleFunc("POST /git/{remote}/git-receive-pack", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
		setGitCacheHeaders(w)
		runGitService(w, r, repo, receivePackService, false)
	})
	return mux
}

type userInput struct {
	Handle      *string `json:"handle"`
	DisplayName *string `json:"display_name"`
}

func registerUserRoutes(mux *http.ServeMux, store *users.Store) {
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		var input userInput
		if err := decodeJSON(r, &input); err != nil || input.Handle == nil || input.DisplayName == nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "handle and display_name are required")
			return
		}
		user, err := store.Create(*input.Handle, *input.DisplayName)
		if writeUserError(w, err) {
			return
		}
		w.Header().Set("Location", "/users/"+user.ID)
		writeJSON(w, http.StatusCreated, user)
	})
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		user, err := store.Get(r.PathValue("id"))
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
	mux.HandleFunc("PATCH /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input userInput
		if err := decodeJSON(r, &input); err != nil || (input.Handle == nil && input.DisplayName == nil) {
			writeAPIError(w, http.StatusBadRequest, "invalid_request", "at least one of handle or display_name is required")
			return
		}
		current, err := store.Get(r.PathValue("id"))
		if writeUserError(w, err) {
			return
		}
		if input.Handle != nil {
			current.Handle = *input.Handle
		}
		if input.DisplayName != nil {
			current.DisplayName = *input.DisplayName
		}
		user, err := store.Update(current.ID, current.Handle, current.DisplayName)
		if writeUserError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, user)
	})
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeUserError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, users.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, users.ErrHandleTaken):
		writeAPIError(w, http.StatusConflict, "handle_taken", "handle is already in use")
	case errors.Is(err, users.ErrInvalidProfile):
		writeAPIError(w, http.StatusBadRequest, "invalid_profile", err.Error())
	default:
		log.Printf("user storage: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "user storage unavailable")
	}
	return true
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func openRemoteRepository(w http.ResponseWriter, store *storage.Store, remote string) (*storage.Repository, bool) {
	id, ok := strings.CutSuffix(remote, ".git")
	if !ok || id == "" {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	repo, err := store.Open(id)
	if errors.Is(err, storage.ErrRepositoryNotFound) || errors.Is(err, storage.ErrInvalidID) {
		http.Error(w, "repository not found", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		http.Error(w, "repository unavailable", http.StatusInternalServerError)
		return nil, false
	}
	return repo, true
}

func runUploadPack(w http.ResponseWriter, r *http.Request, repo *storage.Repository, advertise bool) {
	runGitService(w, r, repo, uploadPackService, advertise)
}

func runGitService(w http.ResponseWriter, r *http.Request, repo *storage.Repository, service string, advertise bool) {
	commandName := strings.TrimPrefix(service, "git-")
	args := []string{commandName, "--stateless-rpc"}
	var removeHooks func()
	if service == receivePackService {
		// Receive-pack applies each requested ref update transactionally. The
		// client distinguishes ordinary progress from explicit force updates,
		// while the hook keeps the remote constrained to its primary branch.
		args = append([]string{
			"-c", "receive.denyNonFastForwards=false",
			"-c", "receive.denyDeletes=false",
			"-c", "receive.denyDeleteCurrent=ignore",
			"-c", "receive.hideRefs=refs/",
			"-c", "receive.hideRefs=!refs/heads/main",
		}, args...)
		if !advertise {
			hooksPath, err := os.MkdirTemp("", "vivarium-receive-hooks-")
			if err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			removeHooks = func() { _ = os.RemoveAll(hooksPath) }
			defer removeHooks()
			if err := os.WriteFile(hooksPath+"/pre-receive", []byte(primaryBranchHook), 0o700); err != nil {
				log.Printf("prepare %s for repository %s: %v", service, repo.ID(), err)
				return
			}
			args = append([]string{"-c", "core.hooksPath=" + hooksPath}, args...)
		}
	}
	if advertise {
		args = append(args, "--advertise-refs")
	}
	args = append(args, repo.Path())
	command := exec.CommandContext(r.Context(), "git", args...)
	// Git services can spawn pack processes. Give the invocation a dedicated
	// process group and cancel the whole group so descendants cannot outlive an
	// abandoned HTTP request.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.Stdout = w
	command.Stderr = os.Stderr
	if !advertise {
		command.Stdin = r.Body
	}
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" && !strings.ContainsAny(protocol, "\x00\r\n") {
		command.Env = append(os.Environ(), "GIT_PROTOCOL="+protocol)
	}
	if err := command.Run(); err != nil {
		log.Printf("serve %s for repository %s: %v", service, repo.ID(), err)
	}
}

func pktLine(payload string) string {
	return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}

func setGitCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
	w.Header().Set("Pragma", "no-cache")
}
