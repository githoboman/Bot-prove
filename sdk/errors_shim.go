package sdk

import "errors"

// goErrorsAs is a tiny shim so tests can call errors.As without polluting
// each test file's imports.
func goErrorsAs(err error, target any) bool { return errors.As(err, target) }
