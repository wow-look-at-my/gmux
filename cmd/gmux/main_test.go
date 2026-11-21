package main

import (
	"bytes"
	"testing"
)

func TestProbe(t *testing.T) {
	var b bytes.Buffer
	probe(&b)
	t.Fatal(b.String())
}
