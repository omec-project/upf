package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

func Test_generate(t *testing.T) {
	interpreter := interp.New(interp.Options{})

	err := interpreter.Use(stdlib.Symbols)
	require.NoError(t, err)

}
