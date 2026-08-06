package storage

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
)

// TreeEntry is one named edge in a Git tree. Type is derived from Mode and
// verified against the referenced object (except for gitlinks, whose commit
// may intentionally live in another repository).
type TreeEntry struct {
	Name string
	Mode string
	ID   ObjectID
	Type ObjectType
}

// TreePath is an entry reached while recursively walking a tree.
type TreePath struct {
	Path string
	TreeEntry
}

// Commit exposes the graph-bearing fields and message of a commit object.
// Headers retains all headers, including author, committer, and signatures,
// in their original order; continuation lines are included in each value.
type Commit struct {
	ID      ObjectID
	Tree    ObjectID
	Parents []ObjectID
	Headers []CommitHeader
	Message []byte
}

// CommitHeader is one commit header and its uninterpreted value.
type CommitHeader struct {
	Name  string
	Value string
}

// ReadTree parses and verifies the direct entries of a tree object.
func (r *Repository) ReadTree(id ObjectID) ([]TreeEntry, error) {
	object, err := r.ReadObject(id)
	if err != nil {
		return nil, err
	}
	if object.Type != TreeObject {
		return nil, graphError(id, fmt.Errorf("expected tree, found %s", object.Type))
	}

	var entries []TreeEntry
	content := object.Content
	for len(content) > 0 {
		space := bytes.IndexByte(content, ' ')
		nul := bytes.IndexByte(content, 0)
		if space <= 0 || nul <= space+1 || len(content) < nul+21 {
			return nil, graphError(id, errors.New("malformed tree entry"))
		}
		mode := string(content[:space])
		name := string(content[space+1 : nul])
		kind, ok := treeEntryType(mode)
		if !ok || !validTreeName(name) {
			return nil, graphError(id, errors.New("invalid tree entry"))
		}
		entryID := ObjectID(fmt.Sprintf("%x", content[nul+1:nul+21]))
		entry := TreeEntry{Name: name, Mode: mode, ID: entryID, Type: kind}
		if mode != "160000" {
			target, readErr := r.ReadObject(entryID)
			if readErr != nil {
				return nil, graphError(id, fmt.Errorf("entry %q: %w", name, readErr))
			}
			if target.Type != kind {
				return nil, graphError(id, fmt.Errorf("entry %q mode names %s but object is %s", name, kind, target.Type))
			}
		}
		entries = append(entries, entry)
		content = content[nul+21:]
	}
	return entries, nil
}

// WalkTree recursively returns every entry in depth-first, tree-storage order.
// Directory entries are included before their descendants.
func (r *Repository) WalkTree(id ObjectID) ([]TreePath, error) {
	var result []TreePath
	active := make(map[ObjectID]bool)
	var walk func(ObjectID, string) error
	walk = func(treeID ObjectID, prefix string) error {
		if active[treeID] {
			return graphError(treeID, errors.New("tree cycle"))
		}
		active[treeID] = true
		defer delete(active, treeID)
		entries, err := r.ReadTree(treeID)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			entryPath := entry.Name
			if prefix != "" {
				entryPath = path.Join(prefix, entry.Name)
			}
			result = append(result, TreePath{Path: entryPath, TreeEntry: entry})
			if entry.Type == TreeObject {
				if err := walk(entry.ID, entryPath); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(id, ""); err != nil {
		return nil, err
	}
	return result, nil
}

// ReadCommit parses the snapshot and parent edges from a commit object.
func (r *Repository) ReadCommit(id ObjectID) (Commit, error) {
	object, err := r.ReadObject(id)
	if err != nil {
		return Commit{}, err
	}
	if object.Type != CommitObject {
		return Commit{}, graphError(id, fmt.Errorf("expected commit, found %s", object.Type))
	}
	headerBytes, message, found := bytes.Cut(object.Content, []byte("\n\n"))
	if !found {
		return Commit{}, graphError(id, errors.New("commit has no header separator"))
	}
	commit := Commit{ID: id, Message: bytes.Clone(message)}
	for _, line := range strings.Split(string(headerBytes), "\n") {
		if strings.HasPrefix(line, " ") {
			if len(commit.Headers) == 0 {
				return Commit{}, graphError(id, errors.New("orphan header continuation"))
			}
			commit.Headers[len(commit.Headers)-1].Value += "\n" + line
			continue
		}
		name, value, ok := strings.Cut(line, " ")
		if !ok || name == "" || value == "" {
			return Commit{}, graphError(id, errors.New("malformed commit header"))
		}
		commit.Headers = append(commit.Headers, CommitHeader{Name: name, Value: value})
		switch name {
		case "tree":
			if commit.Tree != "" || !validObjectID(ObjectID(value)) {
				return Commit{}, graphError(id, errors.New("invalid tree header"))
			}
			commit.Tree = ObjectID(value)
		case "parent":
			if !validObjectID(ObjectID(value)) {
				return Commit{}, graphError(id, errors.New("invalid parent header"))
			}
			commit.Parents = append(commit.Parents, ObjectID(value))
		}
	}
	if commit.Tree == "" {
		return Commit{}, graphError(id, errors.New("missing tree header"))
	}
	if _, err := r.ReadTree(commit.Tree); err != nil {
		return Commit{}, graphError(id, fmt.Errorf("tree %s: %w", commit.Tree, err))
	}
	return commit, nil
}

// ListCommitAncestry walks a commit and all of its parents depth-first in the
// order parent headers appear. Merge ancestors are returned only once.
func (r *Repository) ListCommitAncestry(start ObjectID) ([]Commit, error) {
	var commits []Commit
	seen := make(map[ObjectID]bool)
	var visit func(ObjectID) error
	visit = func(id ObjectID) error {
		if seen[id] {
			return nil
		}
		commit, err := r.ReadCommit(id)
		if err != nil {
			return err
		}
		seen[id] = true
		commits = append(commits, commit)
		for _, parent := range commit.Parents {
			if err := visit(parent); err != nil {
				return graphError(id, fmt.Errorf("parent %s: %w", parent, err))
			}
		}
		return nil
	}
	if err := visit(start); err != nil {
		return nil, err
	}
	return commits, nil
}

func treeEntryType(mode string) (ObjectType, bool) {
	switch mode {
	case "40000", "040000":
		return TreeObject, true
	case "100644", "100755", "120000":
		return BlobObject, true
	case "160000":
		return CommitObject, true
	default:
		return "", false
	}
}

func validTreeName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "\x00/")
}

func graphError(id ObjectID, err error) error {
	return fmt.Errorf("object %s graph: %w: %v", id, ErrCorruptObject, err)
}
