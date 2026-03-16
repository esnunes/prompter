package x

func TransformKeys[T any](m map[string]T, fn func(string) string) map[string]T {
	out := make(map[string]T, len(m))
	for k, v := range m {
		out[fn(k)] = v
	}
	return out
}
