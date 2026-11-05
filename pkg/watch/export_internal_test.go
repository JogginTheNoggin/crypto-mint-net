package watch

// SetExportLimit lowers the entries of one export page for a test, and
// returns a function that puts it back.
func SetExportLimit(n int) func() {
	old := exportLimit
	exportLimit = n
	return func() { exportLimit = old }
}
