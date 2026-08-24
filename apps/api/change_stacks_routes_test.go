package main

import "testing"

func TestChangeStackCycleRemainsExplicit(t *testing.T) {
	graph := map[string][]string{"one": {"two"}, "two": {"one"}}
	if !stackCycle("one", graph, map[string]bool{}, map[string]bool{}) {
		t.Fatal("cycle was hidden")
	}
	if stackCycle("one", map[string][]string{"one": nil}, map[string]bool{}, map[string]bool{}) {
		t.Fatal("acyclic member was blocked")
	}
}
