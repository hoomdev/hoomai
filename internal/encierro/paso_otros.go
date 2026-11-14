//go:build !linux

package encierro

// paso does nothing outside Linux: Envolver never launches the step there.
func paso() {}
