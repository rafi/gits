package domain

import (
	"slices"
	"sort"
	"strings"
)

// Label is a case-insensitive name a Project or Repository carries, used to
// address repositories across projects.
type Label = string

// NormalizeLabel trims and lower-cases label.
func NormalizeLabel(label string) Label {
	return strings.ToLower(strings.TrimSpace(label))
}

// LabelSet is a comparable set of labels. The zero value matches everything.
type LabelSet struct {
	joined string
}

// NewLabelSet builds a set from values, splitting each on commas.
func NewLabelSet(values []string) LabelSet {
	labels := make([]Label, 0, len(values))
	for _, value := range values {
		labels = append(labels, strings.Split(value, ",")...)
	}
	return LabelSet{joined: strings.Join(MergeLabels(labels), ",")}
}

// Empty reports whether the set has no labels.
func (s LabelSet) Empty() bool { return s.joined == "" }

// Names returns the sorted labels in the set.
func (s LabelSet) Names() []Label {
	if s.Empty() {
		return nil
	}
	return strings.Split(s.joined, ",")
}

// String returns the labels comma-separated.
func (s LabelSet) String() string { return s.joined }

// Matches reports whether labels contains any label in the set. An empty set
// matches everything.
func (s LabelSet) Matches(labels []Label) bool {
	if s.Empty() {
		return true
	}
	want := s.Names()
	for _, label := range labels {
		if slices.Contains(want, NormalizeLabel(label)) {
			return true
		}
	}
	return false
}

// MergeLabels returns the normalized, sorted, deduplicated union of lists.
func MergeLabels(lists ...[]Label) []Label {
	var merged []Label
	for _, list := range lists {
		for _, raw := range list {
			label := NormalizeLabel(raw)
			if label == "" || slices.Contains(merged, label) {
				continue
			}
			merged = append(merged, label)
		}
	}
	sort.Strings(merged)
	return merged
}
