package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

const uploadPackService = "git-upload-pack"

func main() {
	root := os.Getenv("GIT_STORAGE_ROOT")
	if root == "" {
		root = "repositories"
	}
	store, err := storage.New(root)
	if err != nil {
		log.Fatal(err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, newHandler(store)); err != nil {
		log.Fatal(err)
	}
}

func newHandler(store *storage.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /git/{remote}/info/refs", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("service") != uploadPackService {
			http.Error(w, "unsupported Git service", http.StatusBadRequest)
			return
		}
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		setGitCacheHeaders(w)
		if _, err := io.WriteString(w, pktLine("# service=git-upload-pack\n")+"0000"); err != nil {
			return
		}
		runUploadPack(w, r, repo, true)
	})
	mux.HandleFunc("POST /git/{remote}/git-upload-pack", func(w http.ResponseWriter, r *http.Request) {
		repo, ok := openRemoteRepository(w, store, r.PathValue("remote"))
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
		setGitCacheHeaders(w)
		runUploadPack(w, r, repo, false)
	})
	return mux
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
	args := []string{"upload-pack", "--stateless-rpc"}
	if advertise {
		args = append(args, "--advertise-refs")
	}
	args = append(args, repo.Path())
	command := exec.Command("git", args...)
	command.Stdout = w
	command.Stderr = os.Stderr
	if !advertise {
		command.Stdin = r.Body
	}
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" && !strings.ContainsAny(protocol, "\x00\r\n") {
		command.Env = append(os.Environ(), "GIT_PROTOCOL="+protocol)
	}
	if err := command.Run(); err != nil {
		log.Printf("serve %s for repository %s: %v", uploadPackService, repo.ID(), err)
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
