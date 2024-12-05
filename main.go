package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Pipeline[T any] interface {
	Process(in <-chan T) <-chan T
}

type Filter struct {
	predicate func(int) bool
	name      string
}

func (f Filter) Process(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for num := range in {
			if f.predicate(num) {
				log.Printf("Фильтр '%s': пропущено число %d", f.name, num)
				out <- num
			} else {
				log.Printf("Фильтр '%s': отфильтровано число %d", f.name, num)
			}
		}
	}()
	return out
}

type Buffer struct {
	size     int
	interval time.Duration
}

func (b Buffer) Process(in <-chan int) <-chan int {
	out := make(chan int)
	buffer := make([]int, 0, b.size)

	go func() {
		defer close(out)
		ticker := time.NewTicker(b.interval)
		defer ticker.Stop()

		for {
			select {
			case num, ok := <-in:
				if !ok {
					for _, n := range buffer {
						out <- n
					}
					return
				}
				if len(buffer) < b.size {
					buffer = append(buffer, num)
					log.Printf("Буфер: добавлено число %d, размер буфера %d/%d",
						num, len(buffer), b.size)
				}
			case <-ticker.C:
				if len(buffer) > 0 {
					for _, num := range buffer {
						out <- num
						log.Printf("Буфер: отправлено число %d", num)
					}
					buffer = buffer[:0]
				}
			}
		}
	}()
	return out
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetPrefix("Pipeline: ")

	input := make(chan int)
	const (
		bufferSize    = 5
		flushInterval = 2 * time.Second
	)

	stage1 := Filter{
		predicate: func(n int) bool { return n >= 0 },
		name:      "положительные числа",
	}
	stage2 := Filter{
		predicate: func(n int) bool { return n != 0 && n%3 == 0 },
		name:      "кратные трем",
	}
	stage3 := Buffer{
		size:     bufferSize,
		interval: flushInterval,
	}

	pipe1 := stage1.Process(input)
	pipe2 := stage2.Process(pipe1)
	pipe3 := stage3.Process(pipe2)

	go processInput(input)

	for num := range pipe3 {
		fmt.Printf("Получено число: %d\n", num)
	}
}

func processInput(input chan<- int) {
	defer close(input)
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("Введите целые числа (для завершения введите 'exit'):")
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "exit" {
			log.Println("Получена команда завершения")
			return
		}

		num, err := strconv.Atoi(text)
		if err != nil {
			fmt.Println("Ошибка: введите корректное целое число")
			continue
		}
		input <- num
	}
}
