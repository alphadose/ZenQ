package zenq

type Publisher interface {
	RegisterSubscriber(any)
}

// NewSelector returns a selector which can listen to multiple ZenQ's
func NewSelector() *ZenQ[any] {
	return new(ZenQ[any])
}

func (self *ZenQ[any]) Subscribe(publishers ...Publisher) *ZenQ[any] {
	for _, pub := range publishers {
		pub.RegisterSubscriber(self)
	}
	return self
}

func (self *ZenQ[any]) Send(value any) {
	self.Write(value)
}

func (self *ZenQ[T]) RegisterSubscriber(sus any) {
	if s, ok := sus.(*ZenQ[any]); ok {
		self.subscriber = s
	} else {
		panic("ZenQ: invalid subscriber type provided to selector")
	}
}
