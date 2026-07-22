//go:build !windows

package scanner

import "os"

func attachProcessLimit(_ *os.Process, _ int) (func(), error) {
	return nil, nil
}
