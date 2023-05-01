package ctftime_test

import (
	"fmt"
	"testing"

	"github.com/dimasma0305/ctfify/function/ctftime"
)

func TestEvents(t *testing.T) {
	result, _ := ctftime.Init().GetEventsByDate(100000, "May 2, 2020", "May 2, 2023")
	filtered := result.Filter(func(chall *ctftime.Event) bool {
		return chall.Weight >= 100.0
	})
	for _, i := range filtered {
		fmt.Println(i.Title)
	}
}
