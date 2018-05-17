package utils

import (
	"fmt"
)

var KeyWord string

func SetKeyword (kw string) {
	fmt.Println(kw)
	KeyWord = kw
}

func GetKeyword () string {
	return KeyWord 
}