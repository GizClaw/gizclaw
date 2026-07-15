//go:build darwin

package i18n

/*
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
char *gizclawPreferredLocale(void);
*/
import "C"

import "unsafe"

func platformLocale() string {
	value := C.gizclawPreferredLocale()
	if value == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(value))
	return C.GoString(value)
}
