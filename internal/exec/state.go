package exec

// DeriveExecutionStatus combines host-run statuses into the execution status.
// Cancellation wins; otherwise all-success, all-failed or mixed.
func DeriveExecutionStatus(statuses []string) string {
	if len(statuses) == 0 {
		return "success"
	}
	var success, failed, cancelled int
	for _, s := range statuses {
		switch s {
		case "success":
			success++
		case "failed":
			failed++
		case "cancelled":
			cancelled++
		}
	}
	if cancelled > 0 {
		return "cancelled"
	}
	switch {
	case failed == 0:
		return "success"
	case success == 0:
		return "failed"
	default:
		return "partial"
	}
}

// DeriveHostRunStatus combines step statuses into a host-run status. Skipped
// steps are ignored.
func DeriveHostRunStatus(statuses []string) string {
	var failed, cancelled, success int
	for _, s := range statuses {
		switch s {
		case "failed":
			failed++
		case "cancelled":
			cancelled++
		case "success":
			success++
		}
	}
	if cancelled > 0 {
		return "cancelled"
	}
	if failed > 0 {
		return "failed"
	}
	if success == 0 {
		return "failed"
	}
	return "success"
}
