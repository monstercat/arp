package main

import "testing"

func Test_main(t *testing.T) {

	root := "/Users/derrick/projects/src/github.com/monstercat/lm/tests"
	fixtures := "/Users/derrick/projects/src/github.com/monstercat/lm/tests/fixtures.yaml"
	colorize := true
	threads := 1
	short := false
	runTests(ProgramArgs{
		TestRoot:    &root,
		Fixtures:    &fixtures,
		Colorize:    &colorize,
		Threads:     &threads,
		Short:       &short,
		Interactive: &short,
		ShortErrors: &short,
		Tiny:        &short,
	})
}
