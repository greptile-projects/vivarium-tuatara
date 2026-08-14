// Package localization persists revision-exact localization extraction and translation work.
package localization

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("localization review not found")
var ErrInvalid = errors.New("invalid localization review")

type ExtractionMap struct {
	ID      string   `json:"id"`
	Version int      `json:"version"`
	Name    string   `json:"name"`
	Include []string `json:"include"`
	Formats []string `json:"formats"`
}
type Variable struct {
	Name    string `json:"name"`
	Example string `json:"example"`
}
type Location struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Component string `json:"component,omitempty"`
}
type Unit struct {
	ID           string            `json:"id"`
	Key          string            `json:"key"`
	Message      string            `json:"message"`
	Context      string            `json:"context"`
	Screenshot   string            `json:"screenshot,omitempty"`
	Variables    []Variable        `json:"variables"`
	PluralRule   string            `json:"plural_rule,omitempty"`
	Locations    []Location        `json:"source_locations"`
	SourceHash   string            `json:"source_hash"`
	Change       string            `json:"change"`
	LocaleStatus map[string]string `json:"locale_status"`
}
type Extraction struct {
	ID             string        `json:"id"`
	PullID         string        `json:"pull_id"`
	SourceRevision string        `json:"source_revision"`
	Map            ExtractionMap `json:"map"`
	Locales        []string      `json:"locales"`
	Units          []Unit        `json:"units"`
	Removed        []Unit        `json:"removed_units"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}
type Translation struct {
	ID         string    `json:"id"`
	UnitID     string    `json:"unit_id"`
	Locale     string    `json:"locale"`
	SourceHash string    `json:"source_hash"`
	Text       string    `json:"text"`
	Note       string    `json:"note,omitempty"`
	Status     string    `json:"status"`
	ProposedBy string    `json:"proposed_by"`
	CreatedAt  time.Time `json:"created_at"`
}
type Review struct {
	RepositoryID    string                    `json:"repository_id"`
	PullID          string                    `json:"pull_id"`
	CurrentRevision string                    `json:"current_revision"`
	Extractions     []Extraction              `json:"extractions"`
	Translations    []Translation             `json:"translations"`
	Counts          map[string]map[string]int `json:"counts"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) Extract(repo, pull, revision, actor string, m ExtractionMap, locales []string, units []Unit) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, pull)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Review{}, err
	}
	if len(revision) != 40 || strings.TrimSpace(m.ID) == "" || m.Version < 1 || strings.TrimSpace(m.Name) == "" || len(m.Include) == 0 || len(m.Formats) == 0 || len(locales) == 0 || len(units) == 0 {
		return Review{}, ErrInvalid
	}
	localeSet := map[string]bool{}
	for _, l := range locales {
		if strings.TrimSpace(l) == "" || localeSet[l] {
			return Review{}, ErrInvalid
		}
		localeSet[l] = true
	}
	prior := map[string]Unit{}
	if len(v.Extractions) > 0 {
		for _, u := range v.Extractions[len(v.Extractions)-1].Units {
			prior[u.ID] = u
		}
	}
	seen := map[string]bool{}
	for i := range units {
		u := &units[i]
		if strings.TrimSpace(u.Key) == "" || strings.TrimSpace(u.Message) == "" || strings.TrimSpace(u.Context) == "" || len(u.Locations) == 0 {
			return Review{}, ErrInvalid
		}
		for _, x := range u.Locations {
			if strings.TrimSpace(x.Path) == "" || x.Line < 1 {
				return Review{}, ErrInvalid
			}
		}
		u.ID = stable(m.ID, u.Key)
		if seen[u.ID] {
			return Review{}, ErrInvalid
		}
		seen[u.ID] = true
		u.SourceHash = hashUnit(*u)
		u.Change = "added"
		if old, ok := prior[u.ID]; ok {
			if old.SourceHash == u.SourceHash {
				u.Change = "reused"
			} else {
				u.Change = "changed"
			}
		}
		u.LocaleStatus = map[string]string{}
	}
	removed := []Unit{}
	for id, u := range prior {
		if !seen[id] {
			u.Change = "removed"
			removed = append(removed, u)
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].ID < removed[j].ID })
	e := Extraction{ID: id(), PullID: pull, SourceRevision: revision, Map: m, Locales: locales, Units: units, Removed: removed, CreatedBy: actor, CreatedAt: s.now()}
	v.RepositoryID = repo
	v.PullID = pull
	v.CurrentRevision = revision
	v.Extractions = append(v.Extractions, e)
	s.project(&v)
	if err := s.write(v); err != nil {
		return Review{}, err
	}
	return v, nil
}
func (s *Store) Propose(repo, pull, revision, unitID, locale, text, note, actor string) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, pull)
	if e != nil {
		return v, e
	}
	if v.CurrentRevision != revision || strings.TrimSpace(text) == "" {
		return v, ErrInvalid
	}
	var unit *Unit
	latest := &v.Extractions[len(v.Extractions)-1]
	for i := range latest.Units {
		if latest.Units[i].ID == unitID {
			unit = &latest.Units[i]
		}
	}
	validLocale := false
	for _, l := range latest.Locales {
		validLocale = validLocale || l == locale
	}
	if unit == nil || !validLocale {
		return v, ErrInvalid
	}
	for i := range v.Translations {
		if v.Translations[i].UnitID == unitID && v.Translations[i].Locale == locale && v.Translations[i].Status == "proposed" {
			v.Translations[i].Status = "superseded"
		}
	}
	v.Translations = append(v.Translations, Translation{ID: id(), UnitID: unitID, Locale: locale, SourceHash: unit.SourceHash, Text: text, Note: note, Status: "proposed", ProposedBy: actor, CreatedAt: s.now()})
	s.project(&v)
	if e = s.write(v); e != nil {
		return Review{}, e
	}
	return v, nil
}
func (s *Store) Get(repo, pull, current string) (Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, pull)
	if e != nil {
		return v, e
	}
	v.CurrentRevision = current
	s.project(&v)
	return v, nil
}
func (s *Store) project(v *Review) {
	v.Counts = map[string]map[string]int{}
	if len(v.Extractions) == 0 {
		return
	}
	latest := &v.Extractions[len(v.Extractions)-1]
	current := map[string]map[string]bool{}
	for i := range v.Translations {
		t := &v.Translations[i]
		match := false
		for _, u := range latest.Units {
			match = match || u.ID == t.UnitID && u.SourceHash == t.SourceHash
		}
		if !match && t.Status == "proposed" {
			t.Status = "superseded"
		}
		if t.Status == "proposed" {
			if current[t.UnitID] == nil {
				current[t.UnitID] = map[string]bool{}
			}
			current[t.UnitID][t.Locale] = true
		}
	}
	for i := range latest.Units {
		u := &latest.Units[i]
		u.LocaleStatus = map[string]string{}
		for _, l := range latest.Locales {
			status := "untranslated"
			if current[u.ID][l] {
				status = "proposed"
			}
			u.LocaleStatus[l] = status
			if v.Counts[l] == nil {
				v.Counts[l] = map[string]int{}
			}
			v.Counts[l][u.Change]++
			if status == "untranslated" {
				v.Counts[l][status]++
			}
		}
	}
	for _, u := range latest.Removed {
		for _, l := range latest.Locales {
			if v.Counts[l] == nil {
				v.Counts[l] = map[string]int{}
			}
			v.Counts[l][u.Change]++
		}
	}
}
func (s *Store) read(repo, pull string) (Review, error) {
	var v Review
	b, e := os.ReadFile(filepath.Join(s.root, repo, pull+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) write(v Review) error {
	d := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".localization-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(d, v.PullID+".json"))
	}
	return e
}
func stable(m, k string) string {
	h := sha256.Sum256([]byte(m + "\x00" + k))
	return hex.EncodeToString(h[:16])
}
func hashUnit(u Unit) string {
	u.ID = ""
	u.SourceHash = ""
	u.Change = ""
	u.LocaleStatus = nil
	b, _ := json.Marshal(u)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
