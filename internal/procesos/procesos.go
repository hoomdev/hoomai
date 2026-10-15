// Package procesos keeps what a command launches together, so that ending it
// ends everything it started: the provider of a run and its tools, or a tmux
// probe and whatever it spawned (findings b5e187, d24626 and f8d179).
package procesos
