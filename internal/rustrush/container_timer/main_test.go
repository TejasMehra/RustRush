package main

import "testing"

func TestClockResolutionReturnsPositiveObservedTick(t *testing.T) {
	if resolution := clockResolution(); resolution == 0 {
		t.Fatal("expected a positive empirical monotonic-clock tick")
	}
}
