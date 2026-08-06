package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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
	contents, err := r.prepareReference(reference)
	if err != nil {
		return err
	}
	parent, base, err := r.openReferenceParent(reference.Name, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	lock, err := lockReference(parent, base)
	if err != nil {
		return err
	}
	defer unlinkAt(parent, base+".lock")
	if file, err := openReferenceFile(parent, base); err == nil {
		_ = file.Close()
		_ = lock.Close()
		return fmt.Errorf("reference %q: %w", reference.Name, ErrReferenceExists)
	} else if !errors.Is(err, syscall.ENOENT) {
		_ = lock.Close()
		return fmt.Errorf("inspect reference %q: %w", reference.Name, err)
	}
	return publishReference(lock, parent, base, contents)
}

// UpdateReference atomically replaces an existing reference.
func (r *Repository) UpdateReference(reference Reference) error {
	contents, err := r.prepareReference(reference)
	if err != nil {
		return err
	}
	parent, base, err := r.openReferenceParent(reference.Name, false)
	if errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("reference %q: %w", reference.Name, ErrReferenceNotFound)
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	lock, err := lockReference(parent, base)
	if err != nil {
		return err
	}
	defer unlinkAt(parent, base+".lock")
	file, err := openReferenceFile(parent, base)
	if errors.Is(err, syscall.ENOENT) {
		_ = lock.Close()
		return fmt.Errorf("reference %q: %w", reference.Name, ErrReferenceNotFound)
	}
	if err != nil {
		_ = lock.Close()
		return corruptReference(reference.Name, err)
	}
	_ = file.Close()
	return publishReference(lock, parent, base, contents)
}

// ReadReference reads and validates one loose reference.
func (r *Repository) ReadReference(name string) (Reference, error) {
	parent, base, err := r.openReferenceParent(name, false)
	if errors.Is(err, syscall.ENOENT) {
		return Reference{}, fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	}
	if err != nil {
		return Reference{}, err
	}
	defer parent.Close()
	file, err := openReferenceFile(parent, base)
	if errors.Is(err, syscall.ENOENT) {
		return Reference{}, fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	}
	if err != nil {
		return Reference{}, corruptReference(name, err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, 1025))
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
	root, err := os.Open(r.path)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	defer root.Close()
	refs, err := openDirectoryAt(root, "refs")
	if err != nil {
		return nil, fmt.Errorf("open refs: %w", err)
	}
	if err := walkReferenceNames(refs, "refs", &names); err != nil {
		_ = refs.Close()
		return nil, fmt.Errorf("list references: %w", err)
	}
	_ = refs.Close()
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
	parent, base, err := r.openReferenceParent(name, false)
	if errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	}
	if err != nil {
		return err
	}
	defer parent.Close()
	lock, err := lockReference(parent, base)
	if err != nil {
		return err
	}
	if err := lock.Close(); err != nil {
		_ = unlinkAt(parent, base+".lock")
		return fmt.Errorf("close reference lock: %w", err)
	}
	defer unlinkAt(parent, base+".lock")
	if err := unlinkAt(parent, base); errors.Is(err, syscall.ENOENT) {
		return fmt.Errorf("reference %q: %w", name, ErrReferenceNotFound)
	} else if err != nil {
		return fmt.Errorf("delete reference %q: %w", name, err)
	}
	if err := unlinkAt(parent, base+".lock"); err != nil {
		return fmt.Errorf("remove reference lock: %w", err)
	}
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync reference directory: %w", err)
	}
	return nil
}

func (r *Repository) prepareReference(reference Reference) ([]byte, error) {
	if err := validateReferenceName(reference.Name); err != nil {
		return nil, err
	}
	if reference.Symbolic {
		if !validRefName(reference.Target) {
			return nil, fmt.Errorf("symbolic target %q: %w", reference.Target, ErrInvalidReference)
		}
		return []byte("ref: " + reference.Target + "\n"), nil
	}
	id := ObjectID(reference.Target)
	if !validObjectID(id) {
		return nil, fmt.Errorf("object target %q: %w", reference.Target, ErrInvalidReference)
	}
	if _, err := r.ReadObject(id); err != nil {
		return nil, fmt.Errorf("reference target %s: %w", id, err)
	}
	return []byte(reference.Target + "\n"), nil
}

// openReferenceParent walks from an already-open repository directory. Every
// component is opened with O_NOFOLLOW, so concurrent symlink replacement can
// neither redirect the returned descriptor nor later descriptor-relative I/O.
func (r *Repository) openReferenceParent(name string, create bool) (*os.File, string, error) {
	if err := validateReferenceName(name); err != nil {
		return nil, "", err
	}
	components := strings.Split(name, "/")
	current, err := os.Open(r.path)
	if err != nil {
		return nil, "", fmt.Errorf("open repository: %w", err)
	}
	for _, component := range components[:len(components)-1] {
		next, openErr := openDirectoryAt(current, component)
		if errors.Is(openErr, syscall.ENOENT) && create {
			if err := syscall.Mkdirat(int(current.Fd()), component, 0o755); err != nil && !errors.Is(err, syscall.EEXIST) {
				_ = current.Close()
				return nil, "", fmt.Errorf("create reference directory: %w", err)
			}
			if err := current.Sync(); err != nil {
				_ = current.Close()
				return nil, "", fmt.Errorf("sync reference directory: %w", err)
			}
			next, openErr = openDirectoryAt(current, component)
		}
		if openErr != nil {
			_ = current.Close()
			return nil, "", fmt.Errorf("open reference directory: %w", openErr)
		}
		_ = current.Close()
		current = next
	}
	return current, components[len(components)-1], nil
}

func validateReferenceName(name string) error {
	if name != "HEAD" && !validRefName(name) {
		return fmt.Errorf("reference name %q: %w", name, ErrInvalidReference)
	}
	return nil
}

func validRefName(name string) bool {
	if !strings.HasPrefix(name, "refs/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") || strings.Contains(name, "@{") || strings.Contains(name, "\\") {
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

func openDirectoryAt(parent *os.File, name string) (*os.File, error) {
	fd, err := syscall.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func openReferenceFile(parent *os.File, name string) (*os.File, error) {
	fd, err := syscall.Openat(int(parent.Fd()), name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("reference is not a regular file")
	}
	return file, nil
}

func lockReference(parent *os.File, name string) (*os.File, error) {
	fd, err := syscall.Openat(int(parent.Fd()), name+".lock", syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o644)
	if errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("reference %q: %w", name, ErrReferenceLocked)
	}
	if err != nil {
		return nil, fmt.Errorf("lock reference: %w", err)
	}
	return os.NewFile(uintptr(fd), name+".lock"), nil
}

func publishReference(lock, parent *os.File, name string, contents []byte) error {
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
	if err := syscall.Renameat(int(parent.Fd()), name+".lock", int(parent.Fd()), name); err != nil {
		return fmt.Errorf("publish reference: %w", err)
	}
	if err := parent.Sync(); err != nil {
		return fmt.Errorf("sync reference directory: %w", err)
	}
	return nil
}

func unlinkAt(parent *os.File, name string) error {
	return syscall.Unlinkat(int(parent.Fd()), name)
}

func walkReferenceNames(directory *os.File, prefix string, names *[]string) error {
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := prefix + "/" + entry.Name()
		if entry.IsDir() {
			child, err := openDirectoryAt(directory, entry.Name())
			if err != nil {
				return err
			}
			err = walkReferenceNames(child, name, names)
			_ = child.Close()
			if err != nil {
				return err
			}
		} else if !strings.HasSuffix(name, ".lock") {
			*names = append(*names, filepath.ToSlash(name))
		}
	}
	return nil
}

func validObjectID(id ObjectID) bool { return isLowerHex(string(id), 40) }

func corruptReference(name string, err error) error {
	return fmt.Errorf("reference %q: %w: %v", name, ErrCorruptReference, err)
}
