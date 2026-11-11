package campaign

import (
	"slices"
	"strings"
	"testing"
	"time"

	"marquee/internal/store"
)

func items(every ...int) []Item {
	out := make([]Item, len(every))
	for i, e := range every {
		out[i] = Item{MediaID: int64(i + 1), Every: e}
	}
	return out
}

// names renders a sequence as letters: A, B, C... by item index.
func names(seq []int) string {
	var b strings.Builder
	for _, i := range seq {
		b.WriteByte(byte('A' + i))
	}
	return b.String()
}

func TestSequenceEveryNth(t *testing.T) {
	for _, tc := range []struct {
		every []int
		want  string
	}{
		{[]int{0, 0, 0}, "ABC"},
		// The product (D) as every 3rd picture: 5 others take two rounds so
		// the pattern also holds where the cycle starts again.
		{[]int{0, 0, 0, 3, 0, 0}, "ABDCEDFADBCDEFD"},
		{[]int{0, 0, 2}, "ACBC"},
		{[]int{4, 0}, "BBBA"},
		{[]int{3, 3}, "AB"}, // nothing else to space them out
	} {
		s := Spec{Items: items(tc.every...)}
		if got := names(s.Sequence(1)); got != tc.want {
			t.Errorf("every %v: %s, want %s", tc.every, got, tc.want)
		}
	}

	// Two focus items, every 3rd and every 4th, between 6 others: every
	// item appears and no regular one is lost.
	s := Spec{Items: items(0, 0, 0, 0, 0, 0, 3, 4)}
	seq := s.Sequence(1)
	for i := range 8 {
		if !slices.Contains(seq, i) {
			t.Fatalf("item %d missing in %s", i, names(seq))
		}
	}
	if len(seq) > 4*maxSequence {
		t.Fatalf("runaway sequence: %d", len(seq))
	}
}

func TestSequenceShuffle(t *testing.T) {
	s := Spec{Order: "shuffle", Items: items(0, 0, 0, 0, 0, 0, 0, 0, 3)}
	a, b := s.Sequence(1), s.Sequence(1)
	if !slices.Equal(a, b) {
		t.Fatal("same seed, different order")
	}
	if slices.Equal(a, s.Sequence(2)) {
		t.Error("different seeds, same order")
	}
	// The focus item keeps its rhythm in a shuffled list.
	for i, x := range a {
		if (i%3 == 2) != (x == 8) {
			t.Fatalf("I not every 3rd: %s", names(a))
		}
	}
}

func TestSpecRejectsCrowdedPlaylist(t *testing.T) {
	s := Spec{Fit: "meet", Items: items(0, 2, 2)}
	if err := s.ValidateDraft(time.UTC); err == nil {
		t.Error("two items every 2nd leave no room for the third")
	}
	s.Items = items(0, 2, 3)
	if err := s.ValidateDraft(time.UTC); err != nil {
		t.Error(err)
	}
	s.Order = "random"
	if err := s.ValidateDraft(time.UTC); err == nil {
		t.Error("unknown order accepted")
	}
}

// The index carries the expanded order: the product appears as every 3rd item.
func TestBuildRepeatsFocusItem(t *testing.T) {
	media := map[int64]*store.Media{}
	for i, sha := range []string{"a", "b", "p"} {
		media[int64(i+1)] = &store.Media{ID: int64(i + 1), Kind: "image", SHA256: sha, Ext: "jpg", Status: "ready"}
	}
	pl := mustRule(t, Spec{Items: []Item{{MediaID: 1, DurationS: 5}, {MediaID: 2, DurationS: 5}, {MediaID: 3, DurationS: 5, Every: 3}}})
	b := Builder{Loc: sofia, RefreshSec: 900, MediaURL: func(m *store.Media) string { return m.FileName() }}
	body, _ := b.Build(time.Date(2026, 11, 10, 15, 0, 0, 0, sofia), []Playable{pl}, media)
	var order []string
	for _, line := range strings.Split(string(body), "\n") {
		if i := strings.Index(line, `src="`); i >= 0 {
			order = append(order, line[i+5:i+6])
		}
	}
	if got := strings.Join(order, ""); got != "abp" {
		t.Fatalf("order %s\n%s", got, body)
	}
}
