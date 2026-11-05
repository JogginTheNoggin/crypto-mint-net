package node

// SetBuildID replaces the build a snapshot is written by and checked
// against, for a test, and returns a function that puts it back.
func SetBuildID(id string) (undo func()) {
	old := buildID
	buildID = func() string { return id }
	return func() { buildID = old }
}
