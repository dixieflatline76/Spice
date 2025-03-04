package util

import "sync/atomic"

// SafeInt is safe to use concurrently.
type SafeInt struct {
	value int32
}

// NewSafeInt creates a new SafeInt.
func NewSafeInt() *SafeInt {
	return &SafeInt{}
}

// NewSafeIntWithValue creates a new SafeInt with an initial value.
func NewSafeIntWithValue(initialValue int) *SafeInt {
	return &SafeInt{value: int32(initialValue)}
}

// Increment increments the counter's value and returns the new value.
func (si *SafeInt) Increment() int {
	return int(atomic.AddInt32(&si.value, 1))
}

// Decrement decrements the counter's value and returns the new value.
func (si *SafeInt) Decrement() int {
	return int(atomic.AddInt32(&si.value, -1))
}

// Add adds a delta to the counter's value and returns the new value.
func (si *SafeInt) Add(delta int) int {
	return int(atomic.AddInt32(&si.value, int32(delta)))
}

// Subtract subtracts a delta from the counter's value and returns the new value.
func (si *SafeInt) Subtract(delta int) int {
	return int(atomic.AddInt32(&si.value, -int32(delta)))
}

// Set sets the value of the counter.
func (si *SafeInt) Set(newValue int) {
	atomic.StoreInt32(&si.value, int32(newValue))
}

// Value returns the current value of the counter.
func (si *SafeInt) Value() int {
	return int(atomic.LoadInt32(&si.value))
}

// SafeBool is safe to use concurrently.
type SafeBool struct {
	value int32
}

// NewSafeBool creates a new SafeBool.
func NewSafeBool() *SafeBool {
	return &SafeBool{}
}

// NewSafeBoolWithValue creates a new SafeBool with an initial value.
func NewSafeBoolWithValue(initialValue bool) *SafeBool {
	var intValue int32
	if initialValue {
		intValue = 1
	}
	return &SafeBool{value: intValue}
}

// Set sets the value of the SafeBool and returns the new value.
func (sb *SafeBool) Set(newValue bool) bool {
	var intValue int32
	if newValue {
		intValue = 1
	}
	atomic.StoreInt32(&sb.value, intValue)
	return newValue
}

// Value returns the current value of the SafeBool.
func (sb *SafeBool) Value() bool {
	return atomic.LoadInt32(&sb.value) != 0
}

// Toggle toggles the value of the SafeBool and returns the new value.
func (sb *SafeBool) Toggle() bool {
	return sb.Set(!sb.Value())
}
