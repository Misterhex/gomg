package main

import (
	"testing"
)

func TestReplaceSpecial(t *testing.T) {

	input := "Junjou Drop"

	result := ReplaceSpecial(input)

	if result != input {
		t.Error(result)
	}

	input = "#000000 - Ultra Black 3"

	result = ReplaceSpecial(input)

	if result != "000000 Ultra Black 3" {

		t.Error(result)
	}

	input = "Kapon_(>_<)!"

	result = ReplaceSpecial(input)

	if result != "Kapon" {

		t.Error(result)
	}

	input = "1/2 Love!"

	result = ReplaceSpecial(input)

	if result != "12 Love" {

		t.Error(result)
	}

	input = "8.1 - Yamada Yuusuke Gekijou"

	result = ReplaceSpecial(input)

	if result != "81 Yamada Yuusuke Gekijou" {

		t.Error(result)
	}

	input = "+C Sword and Cornett"

	result = ReplaceSpecial(input)

	if result != "C Sword and Cornett" {

		t.Error(result)
	}
}
