package publication

import (
	"errors"
	"sort"
	"strings"
)

type CatalogEntry struct {
	Manifest Manifest
	State    string
	Wanted   bool
}

func (s *Store) Catalog() ([]CatalogEntry, error) {
	byID := map[ID]CatalogEntry{}
	known, err := s.KnownIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range known {
		m, err := s.LoadKnown(id)
		if err == nil {
			byID[id] = CatalogEntry{m, "known", s.Wanted(id)}
		}
	}
	partial, err := s.PartialIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range partial {
		a, err := s.LoadPartial(id)
		if err == nil {
			byID[id] = CatalogEntry{a.Manifest, "partial", s.Wanted(id)}
		}
	}
	complete, err := s.IDs()
	if err != nil {
		return nil, err
	}
	for _, id := range complete {
		m, err := s.LoadManifest(id)
		if err == nil {
			byID[id] = CatalogEntry{m, "complete", s.Wanted(id)}
		}
	}
	out := make([]CatalogEntry, 0, len(byID))
	for _, entry := range byID {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID.String() < out[j].Manifest.ID.String() })
	return out, nil
}
func (s *Store) Resolve(prefix string) (ID, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if len(prefix) < 4 || len(prefix) > 32 {
		return ID{}, errors.New("publication ID prefix must contain 4-32 hex characters")
	}
	entries, err := s.Catalog()
	if err != nil {
		return ID{}, err
	}
	var matches []ID
	for _, entry := range entries {
		if strings.HasPrefix(entry.Manifest.ID.String(), prefix) {
			matches = append(matches, entry.Manifest.ID)
		}
	}
	if len(matches) == 0 {
		return ID{}, errors.New("publication ID not found")
	}
	if len(matches) > 1 {
		return ID{}, errors.New("publication ID prefix is ambiguous")
	}
	return matches[0], nil
}
