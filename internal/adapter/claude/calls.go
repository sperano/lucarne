package claude

// maxOpenCalls bounds the tool calls remembered while waiting for their
// result. A call evicted, or one from before the log read began, still
// gets its result; only its tool name is unknown when the result comes.
const maxOpenCalls = 4096

// orderCompactFactor bounds the order queue, closed calls included, to this
// many times maxOpenCalls.
const orderCompactFactor = 2

// openCalls maps the ID of each tool call waiting for its result to the
// tool's name, oldest first.
type openCalls struct {
	names map[string]string
	order []string
}

func newOpenCalls() *openCalls { return &openCalls{names: make(map[string]string)} }

// open records a call; the oldest open call goes when maxOpenCalls are.
func (c *openCalls) open(id, name string) {
	if _, ok := c.names[id]; !ok {
		for len(c.names) >= maxOpenCalls {
			c.evictOldest()
		}
		if len(c.order) >= orderCompactFactor*maxOpenCalls {
			c.compact()
		}
		c.order = append(c.order, id)
	}
	c.names[id] = name
}

// compact drops the IDs of closed calls from order.
func (c *openCalls) compact() {
	open := c.order[:0]
	for _, id := range c.order {
		if _, ok := c.names[id]; ok {
			open = append(open, id)
		}
	}
	c.order = open
}

// close forgets a call and returns its tool's name, empty when it is not
// open.
func (c *openCalls) close(id string) string {
	name := c.names[id]
	delete(c.names, id)
	if len(c.names) == 0 {
		c.order = c.order[:0]
	}
	return name
}

// evictOldest forgets the oldest call still open; order also holds the
// IDs of calls closed since, which it skips.
func (c *openCalls) evictOldest() {
	for len(c.order) > 0 {
		id := c.order[0]
		c.order = c.order[1:]
		if _, ok := c.names[id]; ok {
			delete(c.names, id)
			return
		}
	}
}
