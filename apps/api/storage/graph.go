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
	Name string     `json:"name"`
	Mode string     `json:"mode"`
	ID   ObjectID   `json:"id"`
	Type ObjectType `json:"type"`
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
	var previous *TreeEntry
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
		if previous != nil && (previous.Name == entry.Name || compareTreeEntries(*previous, entry) >= 0) {
			return nil, graphError(id, errors.New("tree entries are duplicate or not canonically ordered"))
		}
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
		previous = &entries[len(entries)-1]
		content = content[nul+21:]
	}
	return entries, nil
}

// WalkTree recursively returns every entry in depth-first, tree-storage order.
// Directory entries are included before their descendants.
func (r *Repository) WalkTree(id ObjectID) ([]TreePath, error) {
	var result []TreePath
	type frame struct {
		id      ObjectID
		prefix  string
		entries []TreeEntry
		next    int
	}
	rootEntries, err := r.ReadTree(id)
	if err != nil {
		return nil, err
	}
	stack := []frame{{id: id, entries: rootEntries}}
	active := map[ObjectID]bool{id: true}
	for len(stack) > 0 {
		current := &stack[len(stack)-1]
		if current.next == len(current.entries) {
			delete(active, current.id)
			stack = stack[:len(stack)-1]
			continue
		}
		entry := current.entries[current.next]
		current.next++
		entryPath := entry.Name
		if current.prefix != "" {
			entryPath = path.Join(current.prefix, entry.Name)
		}
		result = append(result, TreePath{Path: entryPath, TreeEntry: entry})
		if entry.Type != TreeObject {
			continue
		}
		if active[entry.ID] {
			return nil, graphError(entry.ID, errors.New("tree cycle"))
		}
		entries, err := r.ReadTree(entry.ID)
		if err != nil {
			return nil, err
		}
		active[entry.ID] = true
		stack = append(stack, frame{id: entry.ID, prefix: entryPath, entries: entries})
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
			if len(commit.Headers) != 1 || !validObjectID(ObjectID(value)) {
				return Commit{}, graphError(id, errors.New("invalid tree header"))
			}
			commit.Tree = ObjectID(value)
		case "parent":
			if commit.Tree == "" || !validObjectID(ObjectID(value)) || len(commit.Headers) != len(commit.Parents)+2 {
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
	type pendingCommit struct {
		id   ObjectID
		from ObjectID
	}
	stack := []pendingCommit{{id: start}}
	for len(stack) > 0 {
		pending := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		id := pending.id
		if seen[id] {
			continue
		}
		commit, err := r.ReadCommit(id)
		if err != nil {
			if pending.from != "" {
				return nil, graphError(pending.from, fmt.Errorf("parent %s: %w", id, err))
			}
			return nil, err
		}
		seen[id] = true
		commits = append(commits, commit)
		for index := len(commit.Parents) - 1; index >= 0; index-- {
			stack = append(stack, pendingCommit{id: commit.Parents[index], from: id})
		}
	}
	return commits, nil
}

// ListCommitAncestryPage walks ancestry in the same deterministic order while
// stopping after one bounded page. after is the last commit returned by the
// preceding page. The boolean reports whether a non-empty cursor was found.
func (r *Repository) ListCommitAncestryPage(start, after ObjectID, limit int) ([]Commit, *ObjectID, bool, error) {
	var commits []Commit
	seen := make(map[ObjectID]bool)
	found := after == ""
	type pendingCommit struct{ id, from ObjectID }
	stack := []pendingCommit{{id: start}}
	for len(stack) > 0 {
		pending := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[pending.id] {
			continue
		}
		commit, err := r.ReadCommit(pending.id)
		if err != nil {
			if pending.from != "" {
				return nil, nil, found, graphError(pending.from, fmt.Errorf("parent %s: %w", pending.id, err))
			}
			return nil, nil, found, err
		}
		seen[pending.id] = true
		if found {
			commits = append(commits, commit)
			if len(commits) > limit {
				next := commits[limit-1].ID
				return commits[:limit], &next, true, nil
			}
		} else if commit.ID == after {
			found = true
		}
		for index := len(commit.Parents) - 1; index >= 0; index-- {
			stack = append(stack, pendingCommit{id: commit.Parents[index], from: commit.ID})
		}
	}
	return commits, nil, found, nil
}

// compareTreeEntries implements Git's base_name_compare ordering: directory
// names compare as though suffixed with '/', while all other names use NUL.
func compareTreeEntries(left, right TreeEntry) int {
	common := min(len(left.Name), len(right.Name))
	if comparison := bytes.Compare([]byte(left.Name[:common]), []byte(right.Name[:common])); comparison != 0 {
		return comparison
	}
	leftTerminator, rightTerminator := byte(0), byte(0)
	if len(left.Name) > common {
		leftTerminator = left.Name[common]
	} else if left.Type == TreeObject {
		leftTerminator = '/'
	}
	if len(right.Name) > common {
		rightTerminator = right.Name[common]
	} else if right.Type == TreeObject {
		rightTerminator = '/'
	}
	return int(leftTerminator) - int(rightTerminator)
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
