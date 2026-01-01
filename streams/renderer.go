package streams

import "context"

type RunMode int

const (
	Loop RunMode = iota
	OneShot
)

type Renderer interface {
	Init() error
	GetRunMode() RunMode
	Render(data []byte) bool
	GetContext() context.Context
}
