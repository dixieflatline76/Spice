package util

import "sync/atomic"

// SafeCounter is safe to use concurrently.
type SafeCounter struct {
	value int32
}

// NewSafeInt creates a new SafeInt.
func NewSafeInt() *SafeCounter {
	return &SafeCounter{}
}

// NewSafeIntWithValue creates a new SafeInt with an initial value.
func NewSafeIntWithValue(initialValue int) *SafeCounter {
	return &SafeCounter{value: int32(initialValue)}
}

// Increment increments the counter's value and returns the new value.
func (si *SafeCounter) Increment() int {
	return int(atomic.AddInt32(&si.value, 1))
}

// Decrement decrements the counter's value and returns the new value.
func (si *SafeCounter) Decrement() int {
	return int(atomic.AddInt32(&si.value, -1))
}

// Add adds a delta to the counter's value and returns the new value.
func (si *SafeCounter) Add(delta int) int {
	return int(atomic.AddInt32(&si.value, int32(delta)))
}

// Subtract subtracts a delta from the counter's value and returns the new value.
func (si *SafeCounter) Subtract(delta int) int {
	return int(atomic.AddInt32(&si.value, -int32(delta)))
}

// Set sets the value of the counter.
func (si *SafeCounter) Set(newValue int) {
	atomic.StoreInt32(&si.value, int32(newValue))
}

// Value returns the current value of the counter.
func (si *SafeCounter) Value() int {
	return int(atomic.LoadInt32(&si.value))
}

// SafeFlag is safe to use concurrently.
type SafeFlag struct {
	value int32
}

// NewSafeBool creates a new SafeBool.
func NewSafeBool() *SafeFlag {
	return &SafeFlag{}
}

// NewSafeBoolWithValue creates a new SafeBool with an initial value.
func NewSafeBoolWithValue(initialValue bool) *SafeFlag {
	var intValue int32
	if initialValue {
		intValue = 1
	}
	return &SafeFlag{value: intValue}
}

// Set sets the value of the SafeBool and returns the new value.
func (sb *SafeFlag) Set(newValue bool) bool {
	var intValue int32
	if newValue {
		intValue = 1
	}
	atomic.StoreInt32(&sb.value, intValue)
	return newValue
}

// Value returns the current value of the SafeBool.
func (sb *SafeFlag) Value() bool {
	return atomic.LoadInt32(&sb.value) != 0
}

// Toggle toggles the value of the SafeBool and returns the new value.
func (sb *SafeFlag) Toggle() bool {
	return sb.Set(!sb.Value())
}
