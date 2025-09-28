package grafana

import "strings"

func formatTitle(name string) string {
	parts := strings.Split(name, "_")
	for i := range parts {
		parts[i] = strings.Title(strings.ToLower(parts[i]))
	}
	return strings.Join(parts, " ")
}
