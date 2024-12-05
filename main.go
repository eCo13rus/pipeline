package main

import (
	"bufio"
	"docker_test/logger"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	bufferSize    = 5
	flushInterval = 2 * time.Second
	bufferTimeout = 500 * time.Millisecond
)

// Pipeline определяет интерфейс для стадий конвейера
type Pipeline[T any] interface {
	Process(in <-chan T) <-chan T
}

// NumberFilter реализует фильтрацию чисел
type NumberFilter struct {
	predicate func(int) bool
	name      string
}

var log *logger.Logger

func init() {
	var err error
	log, err = logger.New("Pipeline: ")
	if err != nil {
		panic("Не удалось инициализировать логгер: " + err.Error())
	}
}

func (f NumberFilter) Process(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for num := range in {
			if f.predicate(num) {
				log.Info("Фильтр '%s': пропущено число %d", f.name, num)
				out <- num
			} else {
				log.Info("Фильтр '%s': отфильтровано число %d", f.name, num)
			}
		}
	}()
	return out
}

// CircularBuffer реализует кольцевой буфер
type CircularBuffer struct {
	data  []int
	size  int
	head  int
	tail  int
	count int
	mu    sync.Mutex
}

func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]int, size),
		size: size,
	}
}

func (b *CircularBuffer) Push(value int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == b.size {
		log.Info("Буфер: переполнение при добавлении числа %d", value)
		return false
	}

	b.data[b.tail] = value
	b.tail = (b.tail + 1) % b.size
	b.count++
	log.Info("Буфер: добавлено число %d, заполнено %d из %d", value, b.count, b.size)
	return true
}

func (b *CircularBuffer) Pop() (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == 0 {
		return 0, false
	}

	value := b.data[b.head]
	b.head = (b.head + 1) % b.size
	b.count--
	log.Info("Буфер: извлечено число %d, осталось %d из %d", value, b.count, b.size)
	return value, true
}

// BufferStage реализует буферизацию в конвейере
type BufferStage struct {
	buffer *CircularBuffer
	ticker *time.Ticker
}

func NewBufferStage(bufferSize int, flushInterval time.Duration) *BufferStage {
	return &BufferStage{
		buffer: NewCircularBuffer(bufferSize),
		ticker: time.NewTicker(flushInterval),
	}
}

func (s *BufferStage) Process(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer func() {
			s.ticker.Stop()
			close(out)
		}()

		for {
			select {
			case num, ok := <-in:
				if !ok {
					s.flushBuffer(out)
					return
				}
				if !s.buffer.Push(num) {
					s.flushBuffer(out)
					s.buffer.Push(num)
				}

			case <-s.ticker.C:
				s.flushBuffer(out)
			}
		}
	}()
	return out
}

func (s *BufferStage) flushBuffer(out chan<- int) {
	timeout := time.After(bufferTimeout)
	for {
		select {
		case <-timeout:
			log.Info("Буфер: превышен таймаут опустошения")
			return
		default:
			if val, exists := s.buffer.Pop(); exists {
				out <- val
			} else {
				return
			}
		}
	}
}

func processUserInput(input chan<- int) {
	defer close(input)
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Введите целые числа (для завершения введите 'exit'):")
	fmt.Println("Пример корректного ввода: 1, -5, 10, 15")

	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "exit" {
			log.Info("Получена команда завершения")
			return
		}

		num, err := strconv.Atoi(text)
		if err != nil {
			log.Error("Некорректный ввод: %s", text)
			fmt.Println("Ошибка: введите корректное целое число")
			continue
		}

		log.Info("Получено число: %d", num)
		input <- num
	}
}

func main() {
	input := make(chan int)

	positiveFilter := NumberFilter{
		predicate: func(n int) bool { return n >= 0 },
		name:      "положительные числа",
	}

	divisibleByThreeFilter := NumberFilter{
		predicate: func(n int) bool { return n != 0 && n%3 == 0 },
		name:      "кратные трем",
	}

	stage1 := positiveFilter.Process(input)
	stage2 := divisibleByThreeFilter.Process(stage1)
	stage3 := NewBufferStage(bufferSize, flushInterval).Process(stage2)

	go processUserInput(input)

	for num := range stage3 {
		fmt.Printf("Получено значение: %d\n", num)
	}
}
