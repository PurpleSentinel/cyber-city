//go:build !unix

package privdrop

import "errors"

// Drop is unsupported off Unix (docs/DECISIONS.md D15).
func Drop(Account) error { return errors.New("privilege dropping is only supported on Unix") }
