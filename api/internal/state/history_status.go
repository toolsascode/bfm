package state

// HistoryStatusIndicatesApplied returns true if a migrations_history status means
// the migration completed successfully. The tracker maps executor "success" to "applied"
// when persisting; both must be treated as applied everywhere we interpret history.
func HistoryStatusIndicatesApplied(status string) bool {
	return status == "success" || status == "applied"
}
