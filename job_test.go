package main

import (
	"testing"
)

func TestExcept(t *testing.T) {

	inputA := [6]string{"A", "B", "C", "D", "F", "G"}

	inputB := [3]string{"B", "C", "E"}

	result := Except(inputA[:], inputB[:])

	isValid := (len(result) == 4 && result[0] == "A" && result[1] == "D" && result[2] == "F" && result[3] == "G")

	if !isValid {
		t.Error()
	}
}
