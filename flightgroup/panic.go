package flightgroup

import (
	"bytes"
	"fmt"
	"runtime/debug"
)

// PanicErr wraps a recovered panic value with a stack trace.
type PanicErr struct {
	v     any
	stack []byte
}

func (e PanicErr) Error() string {
	return fmt.Sprintf("%v\n%s", e.v, e.stack)
}

func (e PanicErr) Unwrap() error {
	err, ok := e.v.(error)
	if !ok {
		return nil
	}
	return err
}

func newPanicErr(v any) error {
	stack := debug.Stack()
	if line := bytes.IndexByte(stack, '\n'); line >= 0 {
		stack = stack[line+1:]
	}
	return PanicErr{v: v, stack: stack}
}
