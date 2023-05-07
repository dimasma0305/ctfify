package rctf_test

import (
	"fmt"
	"testing"

	"github.com/dimasma0305/ctfify/function/scraper/rctf"
)

func Test_challs(t *testing.T) {
	res, _ := rctf.Init(
		"https://ctf.sdc.tf/",
		"ldFDO1ztj7PJjmChJ7fvvnqM4byJ9G7PzRXHlJV1zG7kI/UGcme1v7X0HQFZXE7s0f5Re+ag4ljTFVXy8ZcefYuryqJl/NxVLRqc7dqebwvdRpTgzklYhi4tcJ4J",
	)

	challs, _ := res.GetChalls()
	fmt.Println(challs.Data[0].WriteTemplatesToDirDefault("/home/dimas/Documents/App/ctfify"))

}
