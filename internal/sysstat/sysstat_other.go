//go:build !linux

package sysstat

// NewReader returns nil on platforms without a /proc filesystem: the status
// bar shows no system-usage segment there. The nil is a nil Reader interface,
// so callers can test `reader == nil`.
func NewReader() Reader {
	return nil
}
