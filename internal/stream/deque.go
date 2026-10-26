package stream

// minDequeSize is the capacity a deque starts with.
const minDequeSize = 64

// deque is a FIFO queue in a ring that grows as needed; its user bounds
// its length.
type deque[T any] struct {
	items []T
	head  int
	n     int
}

func (d *deque[T]) len() int { return d.n }

// at is the i-th item from the oldest.
func (d *deque[T]) at(i int) T { return d.items[(d.head+i)%len(d.items)] }

func (d *deque[T]) push(v T) {
	if d.n == len(d.items) {
		d.grow()
	}
	d.items[(d.head+d.n)%len(d.items)] = v
	d.n++
}

// pop removes and returns the oldest item.
func (d *deque[T]) pop() T {
	var zero T
	v := d.items[d.head]
	d.items[d.head] = zero
	d.head = (d.head + 1) % len(d.items)
	d.n--
	return v
}

func (d *deque[T]) grow() {
	items := make([]T, max(minDequeSize, 2*len(d.items)))
	for i := range d.n {
		items[i] = d.at(i)
	}
	d.items, d.head = items, 0
}
