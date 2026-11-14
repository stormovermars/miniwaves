//go:build race

package mw

// raceFactor widens the timed limits: the race detector makes this CPU-bound work about 15 times slower.
const raceFactor = 4
