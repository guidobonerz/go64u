package streams

type TwitchRenderer struct {
}

func (d *TwitchRenderer) Init() {

}

func (d *TwitchRenderer) Render(data []byte) bool {
	return true
}

func (d *TwitchRenderer) GetRunMode() RunMode {
	return Loop
}
