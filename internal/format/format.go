// Package format renders raw values the way people read them, so the
// reporting code shares one set of formatters.
package format

import "fmt"

// Bytes renders a byte count with familiar labels: 512 B, 16.0 GB, 1.8 TB.
func Bytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
