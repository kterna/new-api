package model

import "testing"

func TestFilterExcludedChannelIDs(t *testing.T) {
	channelIDs := []int{1, 2, 3}
	excluded := map[int]struct{}{2: {}}

	got := filterExcludedChannelIDs(channelIDs, excluded)
	want := []int{1, 3}

	if len(got) != len(want) {
		t.Fatalf("filtered length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("filtered[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestFilterExcludedAbilities(t *testing.T) {
	abilities := []Ability{
		{ChannelId: 1},
		{ChannelId: 2},
		{ChannelId: 3},
	}
	excluded := map[int]struct{}{1: {}, 3: {}}

	got := filterExcludedAbilities(abilities, excluded)

	if len(got) != 1 {
		t.Fatalf("filtered length = %d, want 1", len(got))
	}
	if got[0].ChannelId != 2 {
		t.Fatalf("remaining channel id = %d, want 2", got[0].ChannelId)
	}
}
