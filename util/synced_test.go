package util

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeCounter(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		sc := NewSafeIntWithValue(10)
		assert.Equal(t, 10, sc.Value())

		assert.Equal(t, 11, sc.Increment())
		assert.Equal(t, 11, sc.Value())

		assert.Equal(t, 10, sc.Decrement())
		assert.Equal(t, 10, sc.Value())

		assert.Equal(t, 15, sc.Add(5))
		assert.Equal(t, 15, sc.Value())

		assert.Equal(t, 12, sc.Subtract(3))
		assert.Equal(t, 12, sc.Value())

		sc.Set(100)
		assert.Equal(t, 100, sc.Value())
	})

	t.Run("Concurrency", func(t *testing.T) {
		sc := NewSafeInt()
		var wg sync.WaitGroup
		iterations := 1000

		wg.Add(iterations)
		for i := 0; i < iterations; i++ {
			go func() {
				defer wg.Done()
				sc.Increment()
			}()
		}
		wg.Wait()
		assert.Equal(t, iterations, sc.Value())
	})
}

func TestSafeFlag(t *testing.T) {
	t.Run("Basic Operations", func(t *testing.T) {
		sf := NewSafeBoolWithValue(true)
		assert.True(t, sf.Value())

		assert.False(t, sf.Set(false))
		assert.False(t, sf.Value())

		assert.True(t, sf.Toggle())
		assert.True(t, sf.Value())
	})

	t.Run("Concurrency", func(t *testing.T) {
		sf := NewSafeBool()
		var wg sync.WaitGroup
		iterations := 100

		// Just ensure no race conditions/panics
		wg.Add(iterations)
		for i := 0; i < iterations; i++ {
			go func() {
				defer wg.Done()
				sf.Toggle()
			}()
		}
		wg.Wait()
	})
}
