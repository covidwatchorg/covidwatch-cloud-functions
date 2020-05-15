package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFirestoreEmulator(t *testing.T) {
	e := newFirestoreEmulator(t)
	assert.NotEqual(t, e.host, "")
}
