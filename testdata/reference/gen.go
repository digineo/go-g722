//go:build ignore

// gen writes testdata/input.pcm: a 3 second test signal at 16 kHz, int16
// little endian. Run it from the repository root:
//
//	go run testdata/reference/gen.go
package main

import (
	"encoding/binary"
	"math"
	"os"
)

const rate = 16000

func main() {
	var pcm []int16
	add := func(seconds float64, f func(t float64, i int) float64) {
		n := int(seconds * rate)
		for i := range n {
			pcm = append(pcm, int16(math.Round(f(float64(i)/rate, i))))
		}
	}
	db := func(level float64) float64 { return 32767 * math.Pow(10, level/20) }

	// Sweep from 50 Hz to 7.9 kHz at -3 dBFS: both sub-bands.
	add(1, func(t float64, _ int) float64 {
		return db(-3) * math.Sin(2*math.Pi*(50*t+(7900-50)/2*t*t))
	})
	// Three tones at once, peaking at -1 dBFS.
	add(0.5, func(t float64, _ int) float64 {
		return db(-1) / 3 * (math.Sin(2*math.Pi*300*t) + math.Sin(2*math.Pi*1700*t) + math.Sin(2*math.Pi*5300*t))
	})
	// Noise from a fixed pseudo-random generator (LCG), about -10 dBFS.
	seed := uint32(1)
	add(0.5, func(float64, int) float64 {
		seed = seed*1664525 + 1013904223
		return db(-10) * (float64(seed>>16)/32768 - 1)
	})
	// Silence, then a quiet tone at -40 dBFS.
	add(0.5, func(float64, int) float64 { return 0 })
	add(0.5, func(t float64, _ int) float64 { return db(-40) * math.Sin(2*math.Pi*1000*t) })

	f, err := os.Create("testdata/input.pcm")
	if err != nil {
		panic(err)
	}
	if err := binary.Write(f, binary.LittleEndian, pcm); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}
