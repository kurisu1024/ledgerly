package config

// TODO
type Config struct {
	ChainSize   int `json:"chain-size"`
	WorkerCount int `json:"worker-count"`
	QueueSize   int `json:"queue-size"`
}

func Default() Config {
	c := Config{
		ChainSize:   10,
		WorkerCount: 2,
	}
	c.QueueSize = c.ChainSize * 2
	return c
}
