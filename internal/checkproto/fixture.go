package checkproto

import "fmt"

// Logical fixture names and bodies depend on the seed, not the per-run
// namespace. The namespace only prevents account/room collisions on reruns.
func fixtureUser(prefix string, seed int64, index int) string {
	return fmt.Sprintf("%s_%d_%d", prefix, seed, index)
}
func fixtureBody(prefix string, seed int64, index int) string {
	return fmt.Sprintf("%s/%d/%d", prefix, seed, index)
}
func fixtureNamespace(name, runID string) string { return name + "_" + runID }
