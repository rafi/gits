package domain

import (
	"slices"
	"sort"
	"strings"
)

// Tag is a case-insensitive label on a Project or Repository. It is not a
// git tag.
type Tag = string

// NormalizeTag trims and lower-cases tag.
func NormalizeTag(tag string) Tag {
	return strings.ToLower(strings.TrimSpace(tag))
}

// TagSet is a comparable set of tags. The zero value matches everything.
type TagSet struct {
	joined string
}

// NewTagSet builds a set from values, splitting each on commas.
func NewTagSet(values []string) TagSet {
	tags := make([]Tag, 0, len(values))
	for _, value := range values {
		tags = append(tags, strings.Split(value, ",")...)
	}
	return TagSet{joined: strings.Join(MergeTags(tags), ",")}
}

// Empty reports whether the set has no tags.
func (s TagSet) Empty() bool { return s.joined == "" }

// Names returns the sorted tags in the set.
func (s TagSet) Names() []Tag {
	if s.Empty() {
		return nil
	}
	return strings.Split(s.joined, ",")
}

// String returns the tags comma-separated.
func (s TagSet) String() string { return s.joined }

// Matches reports whether tags contains any tag in the set. An empty set
// matches everything.
func (s TagSet) Matches(tags []Tag) bool {
	if s.Empty() {
		return true
	}
	want := s.Names()
	for _, tag := range tags {
		if slices.Contains(want, NormalizeTag(tag)) {
			return true
		}
	}
	return false
}

// MergeTags returns the normalized, sorted, deduplicated union of lists.
func MergeTags(lists ...[]Tag) []Tag {
	var merged []Tag
	for _, list := range lists {
		for _, raw := range list {
			tag := NormalizeTag(raw)
			if tag == "" || slices.Contains(merged, tag) {
				continue
			}
			merged = append(merged, tag)
		}
	}
	sort.Strings(merged)
	return merged
}
