package main

import (
	"fmt"
	// "strings"
	"reflect"
)

func main() {
	a := "string"
	fmt.Println(reflect.TypeOf(a[0]))
}
