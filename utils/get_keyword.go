package utils

import (
	"fmt"
)

var KeyWord chan string

func SetKeyword (kw string) string {
	fmt.Println(kw)
	KeyWord <- kw
}

func GetKeyword () string {
	kw := <- KeyWord
	fmt.Println(kw)
	return kw 
}