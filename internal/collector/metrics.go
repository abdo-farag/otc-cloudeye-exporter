package collectors

import (
	"time"
)

type MetricExport struct {
	MetricName string
	Labels     map[string]string
	Value      float64
	Unit       string
	Timestamp  time.Time
}
