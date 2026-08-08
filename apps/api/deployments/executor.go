package deployments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

// Executor is the only boundary that receives decrypted environment values.
// It verifies the immutable artifact immediately before invoking the isolated
// owner-defined delivery command.
type Executor struct {
	store  *Store
	builds *checkruns.Store
	owner  string
	now    func() time.Time
}

func NewExecutor(store *Store, builds *checkruns.Store) *Executor {
	owner, err := newID()
	if err != nil {
		panic("deployment executor identity unavailable")
	}
	return &Executor{store: store, builds: builds, owner: owner, now: time.Now}
}

func (e *Executor) Execute(repositoryID, id string) error {
	promotion, err := e.store.GetPromotion(repositoryID, id)
	if err != nil {
		return err
	}
	environment, err := e.store.ExecutionEnvironment(repositoryID, promotion.EnvironmentID)
	if err != nil {
		return e.fail(promotion, "environment policy is unavailable")
	}
	leaseExpires := e.now().UTC().Add(time.Duration(environment.TimeoutSeconds)*time.Second + time.Minute)
	promotion, err = e.store.Claim(repositoryID, id, e.owner, leaseExpires)
	if err != nil {
		return err
	}
	artifact, metadata, err := e.builds.OpenArtifact(repositoryID, promotion.ReleaseID, promotion.BuildID, promotion.ArtifactID)
	if err != nil {
		return e.fail(promotion, "immutable artifact is unavailable")
	}
	defer artifact.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, artifact); err != nil || hex.EncodeToString(hash.Sum(nil)) != promotion.ArtifactSHA256 || metadata.SHA256 != promotion.ArtifactSHA256 {
		return e.fail(promotion, "artifact checksum verification failed")
	}
	if _, err = artifact.Seek(0, io.SeekStart); err != nil {
		return e.fail(promotion, "immutable artifact cannot be prepared")
	}
	envFile, err := os.CreateTemp("", "vivarium-deployment-env-*")
	if err != nil {
		return e.fail(promotion, "protected environment could not be prepared")
	}
	envName := envFile.Name()
	defer os.Remove(envName)
	if err = envFile.Chmod(0600); err == nil {
		values := make(map[string]string, len(environment.Configuration)+len(environment.Credentials)+1)
		for key, value := range environment.Configuration {
			values[key] = value
		}
		for key, value := range environment.Credentials {
			values[key] = value
		}
		values["VIVARIUM_ARTIFACT"] = "/vivarium/artifact"
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.ContainsAny(values[key], "\r\n\x00") {
				err = errors.New("environment value contains newline")
				break
			}
			_, err = fmt.Fprintf(envFile, "%s=%s\n", key, values[key])
			if err != nil {
				break
			}
		}
	}
	if closeErr := envFile.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return e.fail(promotion, "protected environment could not be prepared")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(environment.TimeoutSeconds)*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "run", "--rm", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m", "--env-file", envName, "--mount", "type=bind,src="+artifact.Name()+",dst=/vivarium/artifact,readonly", environment.Image, "sh", "-c", environment.Command)
	var output limitedBuffer
	command.Stdout, command.Stderr = &output, &output
	err = command.Run()
	log := redact(output.String(), environment.Credentials)
	if ctx.Err() == context.DeadlineExceeded {
		return e.fail(promotion, "deployment command timed out\n"+log)
	}
	if err != nil {
		return e.fail(promotion, "deployment command failed\n"+log)
	}
	_, err = e.store.Complete(repositoryID, promotion.ID, e.owner, "succeeded", "Artifact SHA-256 verified; deployment command completed.\n"+log)
	return err
}

func (e *Executor) Recover() error {
	items, err := e.store.Nonterminal()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.State == "running" {
			if item.LeaseExpiresAt == nil || !item.LeaseExpiresAt.After(e.now().UTC()) {
				_, _ = e.store.Complete(item.RepositoryID, item.ID, item.ExecutionOwner, "failed", "Execution lease expired; external outcome is unknown")
			}
		} else {
			go e.Execute(item.RepositoryID, item.ID)
		}
	}
	return nil
}

func (e *Executor) fail(promotion Promotion, message string) error {
	_, err := e.store.Complete(promotion.RepositoryID, promotion.ID, e.owner, "failed", message)
	return err
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if b.Len() < 64*1024 {
		remaining := 64*1024 - b.Len()
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return original, nil
}
func redact(value string, secrets map[string]string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
