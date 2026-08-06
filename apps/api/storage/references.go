package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Reference is a named Git pointer. Direct references contain a lowercase
// object ID; symbolic references contain the name of another reference.
type Reference struct {
	Name     string
	Target   string
	Symbolic bool
}

// CreateReference creates a reference without replacing an existing one.
func (r *Repository) CreateReference(reference Reference) error {
	path, contents, err := r.prepareReference(reference)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create reference directory: %w", err)
	}
	lock, err := lockReference(path)
	if err != nil {
		return err
	}
	defer os.Remove(lock.Name())
	if _, err := os.Lstat(path); err == nil {
		_ = lock.Close()
		return fmt.Errorf("reference %q: %w", reference.Name, ErrReferenceExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = lock.Close()
		return fmt.Errorf("inspect reference %q: %w", reference.Name, err)
	}
	if err := publishReference(lock, path, contents); err != nil {
		return err
	}
	return r.syncReferenceParents(path)
}

// UpdateReference atomically replaces an existing reference.
func (r *Repository) UpdateReference(reference Reference) error {
	path, contents, err := r.prepareReference(reference)
	if err != nil {
		return err
	}
	lock, err := lockReference(path)
	if err != nil {
		return err
	}
	defer os.Remove(lock.Name())
	if info, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		_ = lock.Close()
		return fmt.Errorf("reference %q: %w", reference.Name, ErrReferenceNotFound)
	} else if err != nil {
		_ = lock.Close()
		return fmt.Errorf("inspect reference %q: %w", reference.Name, err)
	} else if !info.Mode().IsRegular() {
		_ = lock.Close()
		return corruptReference(reference.Name, errors.New("reference is not a regular file"))
	}
	if err := publishReference(lock, path, contents); err != nil {
		return err
	}
	return r.syncReferenceParents(path)
}

// ReadReference reads and validates one loose reference.
func (r *Repository) ReadReference(name string) (Reference, error) {
	path, err := r.referencePath(name)
	if err != nil {
		return Reference{}, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Reference{}, fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	}
	if err != nil {
		return Reference{}, fmt.Errorf("inspect reference %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return Reference{}, corruptReference(name, errors.New("reference is not a regular file"))
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return Reference{}, fmt.Errorf("read reference %q: %w", name, err)
	}
	if len(contents) == 0 || len(contents) > 1024 || contents[len(contents)-1] != '\n' || strings.Count(string(contents), "\n") != 1 {
		return Reference{}, corruptReference(name, errors.New("invalid reference contents"))
	}
	target := strings.TrimSuffix(string(contents), "\n")
	if symbolic, found := strings.CutPrefix(target, "ref: "); found {
		if !validRefName(symbolic) {
			return Reference{}, corruptReference(name, errors.New("invalid symbolic target"))
		}
		return Reference{Name: name, Target: symbolic, Symbolic: true}, nil
	}
	if !validObjectID(ObjectID(target)) {
		return Reference{}, corruptReference(name, errors.New("invalid object ID target"))
	}
	if _, err := r.ReadObject(ObjectID(target)); err != nil {
		return Reference{}, corruptReference(name, err)
	}
	return Reference{Name: name, Target: target}, nil
}

// ListReferences returns HEAD and every loose reference, ordered by name.
func (r *Repository) ListReferences() ([]Reference, error) {
	names := []string{"HEAD"}
	err := filepath.WalkDir(filepath.Join(r.path, "refs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(r.path, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if strings.HasSuffix(name, ".lock") {
			return nil
		}
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list references: %w", err)
	}
	slices.Sort(names)
	references := make([]Reference, 0, len(names))
	for _, name := range names {
		reference, err := r.ReadReference(name)
		if err != nil {
			return nil, fmt.Errorf("enumerate %s: %w", name, err)
		}
		references = append(references, reference)
	}
	return references, nil
}

// DeleteReference atomically coordinates with writers and removes a reference.
func (r *Repository) DeleteReference(name string) error {
	path, err := r.referencePath(name)
	if err != nil {
		return err
	}
	lock, err := lockReference(path)
	if err != nil {
		return err
	}
	lockPath := lock.Name()
	if err := lock.Close(); err != nil {
		_ = os.Remove(lockPath)
		return fmt.Errorf("close reference lock: %w", err)
	}
	defer os.Remove(lockPath)
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	} else if err != nil {
		return fmt.Errorf("delete reference %q: %w", name, err)
	}
	if err := os.Remove(lockPath); err != nil {
		return fmt.Errorf("remove reference lock: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync reference directory: %w", err)
	}
	return r.syncReferenceParents(path)
}

func (r *Repository) prepareReference(reference Reference) (string, []byte, error) {
	path, err := r.referencePath(reference.Name)
	if err != nil {
		return "", nil, err
	}
	if reference.Symbolic {
		if !validRefName(reference.Target) {
			return "", nil, fmt.Errorf("symbolic target %q: %w", reference.Target, ErrInvalidReference)
		}
		return path, []byte("ref: " + reference.Target + "\n"), nil
	}
	id := ObjectID(reference.Target)
	if !validObjectID(id) {
		return "", nil, fmt.Errorf("object target %q: %w", reference.Target, ErrInvalidReference)
	}
	if _, err := r.ReadObject(id); err != nil {
		return "", nil, fmt.Errorf("reference target %s: %w", id, err)
	}
	return path, []byte(reference.Target + "\n"), nil
}

func (r *Repository) referencePath(name string) (string, error) {
	if name != "HEAD" && !validRefName(name) {
		return "", fmt.Errorf("reference name %q: %w", name, ErrInvalidReference)
	}
	return filepath.Join(r.path, filepath.FromSlash(name)), nil
}

func validRefName(name string) bool {
	if !strings.HasPrefix(name, "refs/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.Contains(name, "\\") {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	for _, character := range name {
		if character <= ' ' || character == 0x7f || strings.ContainsRune("~^:?*[", character) {
			return false
		}
	}
	return true
}

func lockReference(path string) (*os.File, error) {
	lock, err := os.OpenFile(path+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("reference %q: %w", filepath.Base(path), ErrReferenceLocked)
	}
	if err != nil {
		return nil, fmt.Errorf("lock reference: %w", err)
	}
	return lock, nil
}

func publishReference(lock *os.File, path string, contents []byte) error {
	lockPath := lock.Name()
	if _, err := lock.Write(contents); err != nil {
		_ = lock.Close()
		return fmt.Errorf("write reference: %w", err)
	}
	if err := lock.Sync(); err != nil {
		_ = lock.Close()
		return fmt.Errorf("sync reference: %w", err)
	}
	if err := lock.Close(); err != nil {
		return fmt.Errorf("close reference: %w", err)
	}
	if err := os.Rename(lockPath, path); err != nil {
		return fmt.Errorf("publish reference: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync reference directory: %w", err)
	}
	return nil
}

func validObjectID(id ObjectID) bool {
	return isLowerHex(string(id), 40)
}

func corruptReference(name string, err error) error {
	return fmt.Errorf("reference %q: %w: %v", name, ErrCorruptReference, err)
}

func (r *Repository) syncReferenceParents(path string) error {
	for directory := filepath.Dir(path); directory != r.path; directory = filepath.Dir(directory) {
		if err := syncDirectory(directory); err != nil {
			return fmt.Errorf("sync reference hierarchy: %w", err)
		}
	}
	return nil
}
