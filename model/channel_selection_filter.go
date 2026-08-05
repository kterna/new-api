package model

func channelSelectionExcludedIDs(excludedChannelIDs ...map[int]struct{}) map[int]struct{} {
	if len(excludedChannelIDs) == 0 {
		return nil
	}
	return excludedChannelIDs[0]
}

func filterExcludedChannelIDs(channelIDs []int, excludedChannelIDs map[int]struct{}) []int {
	if len(channelIDs) == 0 || len(excludedChannelIDs) == 0 {
		return channelIDs
	}

	filtered := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if _, excluded := excludedChannelIDs[channelID]; !excluded {
			filtered = append(filtered, channelID)
		}
	}
	return filtered
}

func filterExcludedAbilities(abilities []Ability, excludedChannelIDs map[int]struct{}) []Ability {
	if len(abilities) == 0 || len(excludedChannelIDs) == 0 {
		return abilities
	}

	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if _, excluded := excludedChannelIDs[ability.ChannelId]; !excluded {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}
