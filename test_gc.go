package main

import (
	"fmt"

	"github.com/tangzhangming/nova/internal/runtime"
)

func main() {
	source := `
print("Hello World!");
$x = 1 + 2;
print("1 + 2 =", $x);
`
	r := runtime.New()
	if err := r.Run(source, "test.sola"); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}


