package prof

import (
	"reflect"
	"testing"
	"time"
)

func TestSortif(t *testing.T) {
	data := []oneval{
		{name: "A", tim: 10},
		{name: "B", tim: 5},
		{name: "C", tim: 15},
	}

	s := sortif{val: data}

	expectedLen := len(data)
	if gotLen := s.Len(); gotLen != expectedLen {
		t.Errorf("Length mismatch, expected: %d, got: %d", expectedLen, gotLen)
	}

	if !s.Less(0, 1) {
		t.Error("Expected first element to be less than the second one")
	}
	if s.Less(1, 0) {
		t.Error("Expected first element to be greater than the second one")
	}

	expectedData := []oneval{
		{name: "B", tim: 5},
		{name: "A", tim: 10},
		{name: "C", tim: 15},
	}
	s.Swap(0, 1)
	if !reflect.DeepEqual(s.val, expectedData) {
		t.Errorf("Swap result mismatch, expected: %+v, got: %+v", expectedData, s.val)
	}
}

func TestStaSto(t *testing.T) {
	Start()

	// Test Sta and Sto functions
	name := "testCheckpoint"
	Sta(name)
	time.Sleep(100 * time.Millisecond)
	Sto(name)
	// If Sta and Sto work correctly, no panic should occur
}

func TestStop(t *testing.T) {
	Start()

	// Test Stop function
	Stop()
	// If Stop works correctly, no panic should occur
}

func TestStart(t *testing.T) {
	// Test Start function
	Start()
	// If Start works correctly, no panic should occur
}

func TestProfilerDisabled(t *testing.T) {
	// Test ProfilerDisabled flag
	if ProfilerDisabled {
		t.Error("Profiler should not be disabled initially")
	}

	ProfilerDisabled = true
	// If ProfilerDisabled is set to true, it should not panic
}
