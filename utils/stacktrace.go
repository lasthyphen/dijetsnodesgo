

package utils

import "runtime"

func GetStacktrace(all bool) string {
	buf := make([]byte, 1<<16)
	n := runtime.Stack(buf, all)
	return string(buf[:n])
}
