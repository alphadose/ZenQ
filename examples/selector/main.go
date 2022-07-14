package main

import (
	"fmt"

	"github.com/alphadose/zenq/v2"
)

type custom1 struct {
	alpha int
	beta  string
}

type custom2 struct {
	gamma int
}

const size = 100

var (
	zq1 = zenq.New[int](size)
	zq2 = zenq.New[string](size)
	zq3 = zenq.New[custom1](size)
	zq4 = zenq.New[*custom2](size)
)

func main() {
	go looper(intProducer)
	go looper(stringProducer)
	go looper(custom1Producer)
	go looper(custom2Producer)

	for i := 0; i < 40; i++ {

		// Selection occurs here
		if data, ok := zenq.Select(zq1, zq2, zq3, zq4); ok {
			switch data.(type) {
			case int:
				fmt.Printf("Received int %d\n", data)
			case string:
				fmt.Printf("Received string %s\n", data)
			case custom1:
				fmt.Printf("Received custom data type number 1 %#v\n", data)
			case *custom2:
				fmt.Printf("Received pointer %#v\n", data)
			}
		}
	}
}

func intProducer(ctr int) { zq1.Write(ctr) }

func stringProducer(ctr int) { zq2.Write(fmt.Sprint(ctr * 10)) }

func custom1Producer(ctr int) { zq3.Write(custom1{alpha: ctr, beta: fmt.Sprint(ctr)}) }

func custom2Producer(ctr int) { zq4.Write(&custom2{gamma: 1 << ctr}) }

func looper(producer func(ctr int)) {
	for i := 0; i < 10; i++ {
		producer(i)
	}
}
