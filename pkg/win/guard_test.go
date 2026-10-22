//go:build windows

package win

import "testing"

// A panic in a handler gives OnPanic its one chance (even if the save
// panics too), then goes on: the program ends rather than carrying on in
// a state the panic left half done.
func TestGuardSavesOnceThenPanics(t *testing.T) {
	calls := 0
	w := &Window{OnPanic: func(r any) {
		calls++
		if r != "boom" {
			t.Errorf("OnPanic got %v", r)
		}
		panic("the save failed too")
	}}
	run := func() (r any) {
		defer func() { r = recover() }()
		func() {
			defer w.guard()
			panic("boom")
		}()
		return nil
	}
	if r := run(); r != "boom" {
		t.Fatalf("the panic didn't go on as it was: %v", r)
	}
	if r := run(); r != "boom" {
		t.Fatalf("second panic: %v", r)
	}
	if calls != 1 {
		t.Fatalf("OnPanic called %d times, want once", calls)
	}
	// no panic: nothing happens
	func() { defer w.guard() }()
}
