package config

// TODO
type Config struct {
	BatchSize   int `json:"batch-size"`
	WorkerCount int `json:"worker-count"`
	QueueSize   int `json:"queue-size"`
}

func Default() Config {
	c := Config{
		BatchSize:   10,
		WorkerCount: 2,
	}
	c.QueueSize = c.BatchSize * 2
	return c
}
