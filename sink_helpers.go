package logger

import "log/slog"

// addAttrToMap recursively adds slog attributes to a map.
// Used by Loki and VictoriaLogs encoders to flatten attributes.
func addAttrToMap(m map[string]any, attr slog.Attr, prefix string) {
	key := attr.Key
	if prefix != "" {
		key = prefix + "." + key
	}

	attr.Value = attr.Value.Resolve()
	switch attr.Value.Kind() {
	case slog.KindGroup:
		for _, groupAttr := range attr.Value.Group() {
			addAttrToMap(m, groupAttr, key)
		}
	default:
		m[key] = attr.Value.Any()
	}
}
