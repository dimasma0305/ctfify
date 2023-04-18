package log

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

func Fatal(err error) {
	text := strings.Split(strings.TrimSpace(err.Error()), "\n")
	for _, v := range text {
		fmt.Fprintln(os.Stderr, color.RedString("[x] ")+v)
	}
	os.Exit(1)
}

func Info(format string, elem ...any) {
	fmt.Println(color.BlueString("[x] ") + fmt.Sprintf(format, elem...))
}

func InfoH2(format string, elem ...any) {
	fmt.Println(color.GreenString("  [x] ") + fmt.Sprintf(format, elem...))
}

func InfoH3(format string, elem ...any) {
	fmt.Println(color.YellowString("    [x] ") + fmt.Sprintf(format, elem...))
}
