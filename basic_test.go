package main

import (
	"fmt"
	"testing"
)

func TestStringByteConvert(t *testing.T) {
	a := []byte("123")

	fmt.Println(string(a))
	fmt.Println(a[0])
	fmt.Println([]byte(string(a)))

}
