// Code generated by "stringer -type=formatCode"; DO NOT EDIT

package pgwire

import "fmt"

const _formatCode_name = "formatTextformatBinary"

var _formatCode_index = [...]uint8{0, 10, 22}

func (i formatCode) String() string {
	if i < 0 || i >= formatCode(len(_formatCode_index)-1) {
		return fmt.Sprintf("formatCode(%d)", i)
	}
	return _formatCode_name[_formatCode_index[i]:_formatCode_index[i+1]]
}
