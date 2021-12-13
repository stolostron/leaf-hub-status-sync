package bundle

import set "github.com/deckarep/golang-set"

// containsString returns true if the string exists in the array and false otherwise.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}

	return false
}

// createSetFromSlice returns a set contains all items in the given slice. if slice is nil, returns empty set.
func createSetFromSlice(slice []string) set.Set {
	if slice == nil {
		return set.NewSet()
	}

	result := set.NewSet()
	for _, item := range slice {
		result.Add(item)
	}

	return result
}
