package flightgroup

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// FuzzDo exercises the flightgroup with arbitrary keys and concurrent caller counts.
func FuzzDo(f *testing.F) {
	f.Add("key", uint8(1))
	f.Add("", uint8(5))
	f.Add("special-chars-!@#$%^&*()", uint8(10))
	f.Add("unicode-日本語", uint8(3))

	f.Fuzz(func(t *testing.T, key string, numCallers uint8) {
		if numCallers == 0 {
			numCallers = 1
		}
		// Cap to prevent unreasonable goroutine counts
		if numCallers > 50 {
			numCallers = 50
		}

		g := NewGroup[string, string]()
		n := int(numCallers)
		expected := fmt.Sprintf("result-for-%s", key)

		var wg sync.WaitGroup
		wg.Add(n)
		for i := range n {
			go func(tag int) {
				defer wg.Done()
				order := g.Do(key, uint64(tag), func() (string, error) {
					return expected, nil
				})
				result := <-order.Ch()
				assert.NoError(t, result.Err)
				assert.Equal(t, expected, result.Value)
			}(i)
		}
		wg.Wait()
	})
}

// FuzzDoError verifies error propagation with arbitrary error messages.
func FuzzDoError(f *testing.F) {
	f.Add("key", "something went wrong")
	f.Add("", "")
	f.Add("k", "error with special chars: \n\t\x00")
	f.Fuzz(func(t *testing.T, key, errMsg string) {
		g := NewGroup[string, int]()
		sentinel := fmt.Errorf("%s", errMsg)

		order := g.Do(key, 0, func() (int, error) {
			return 0, sentinel
		})
		result := <-order.Ch()
		assert.Error(t, result.Err)
		assert.True(t, errors.Is(result.Err, sentinel), "expected error to wrap sentinel")
	})
}
