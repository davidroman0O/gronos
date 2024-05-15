package gronos

/// nothing fancy, it is just an incremental id generator

type flipID struct {
	current uint
}

func newFlipID() *flipID {
	return &flipID{
		current: 1, // 0 means no id
	}
}

func (f *flipID) Next() uint {
	f.current++
	return f.current
}
