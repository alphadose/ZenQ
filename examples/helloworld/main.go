package main

import (
	"fmt"
	"github.com/alphadose/zenq"
)

type payload struct {
	beta string
}

func main() {
	zq := zenq.New[payload]()

	p := payload{}
	for i := 0; i < 10; i++ {
		p.beta = "hello world"
		zq.Write(p)
	}

	r := payload{}
	for i := 0; i < 10; i++ {
		r = zq.Read()
		fmt.Println(i, r.beta)
	}
}
