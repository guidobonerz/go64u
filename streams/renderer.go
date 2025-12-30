package streams

type RunMode int

const (
	Loop RunMode = 0
	OneShot
)

type Renderer interface {
	Init()
	GetRunMode() RunMode
	Render(data []byte) bool
}
