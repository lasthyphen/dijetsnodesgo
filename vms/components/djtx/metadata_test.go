

package djtx

import (
	"testing"
)

func TestMetaDataVerifyNil(t *testing.T) {
	md := (*Metadata)(nil)
	if err := md.Verify(); err == nil {
		t.Fatalf("Should have errored due to nil metadata")
	}
}

func TestMetaDataVerifyUninitialized(t *testing.T) {
	md := &Metadata{}
	if err := md.Verify(); err == nil {
		t.Fatalf("Should have errored due to uninitialized metadata")
	}
}
