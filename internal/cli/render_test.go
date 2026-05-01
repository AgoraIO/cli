package cli

import (
	"testing"
)

// TestDetectTerminalColumnsHonorsCOLUMNSEnv covers the user-override
// path. COLUMNS is checked first so users (and tests) can force a
// known width without needing a real TTY.
func TestDetectTerminalColumnsHonorsCOLUMNSEnv(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	got := detectTerminalColumns()
	if got != 120 {
		t.Fatalf("detectTerminalColumns() = %d, want 120 from COLUMNS", got)
	}
}

// TestDetectTerminalColumnsIgnoresInvalidCOLUMNS verifies we fall
// through to the term.GetSize / zero path when COLUMNS is garbage,
// rather than coercing it to 0 and accidentally suppressing later
// detection.
func TestDetectTerminalColumnsIgnoresInvalidCOLUMNS(t *testing.T) {
	t.Setenv("COLUMNS", "not-a-number")
	// We can't assert the exact result because it depends on whether
	// stdin/stdout are TTYs in the test runner. The contract is:
	// the function must NOT return a parsed-but-bogus value.
	got := detectTerminalColumns()
	if got < 0 {
		t.Fatalf("detectTerminalColumns() = %d, want >= 0", got)
	}
}

// TestTerminalValueWidthSuppressesNarrowResults is a behavioral test
// for the truncation gate. Below 20 chars of available value-column
// room, truncation produces unreadable output, so the helper must
// return 0 (caller should not truncate).
func TestTerminalValueWidthSuppressesNarrowResults(t *testing.T) {
	t.Setenv("COLUMNS", "30")
	if got := terminalValueWidth(20); got != 0 {
		t.Errorf("terminalValueWidth(20) at COLUMNS=30 = %d, want 0 (too narrow)", got)
	}
	t.Setenv("COLUMNS", "120")
	if got := terminalValueWidth(20); got <= 0 {
		t.Errorf("terminalValueWidth(20) at COLUMNS=120 = %d, want positive", got)
	}
}

// TestTerminalValueWidthZeroWhenWidthUnknown is the "no terminal
// detected" contract. Returning 0 means "do not truncate", which is
// the safest default for log scrapers and CI build logs.
func TestTerminalValueWidthZeroWhenWidthUnknown(t *testing.T) {
	t.Setenv("COLUMNS", "")
	// In CI without a TTY this returns 0; on developer machines with
	// a TTY this may return a positive value. We only assert
	// non-negative because the contract is "no panic, sensible
	// fallback".
	got := terminalValueWidth(10)
	if got < 0 {
		t.Fatalf("terminalValueWidth = %d, want >= 0", got)
	}
}
