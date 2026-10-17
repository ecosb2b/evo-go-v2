package receipt_handler

import (
	"reflect"
	"testing"
)

func TestNormalizeIDs(t *testing.T) {
	got := normalizeIDs([]string{" A ", "", "B", "A", "  ", "C"})
	want := []string{"A", "B", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeIDs = %v, want %v", got, want)
	}
}
