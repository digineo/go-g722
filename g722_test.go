package g722

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The files in testdata come from testdata/reference/gen.go (input) and
// the original C implementation via testdata/reference/ref.c (expected
// results).

func readPCM(t *testing.T, name string) []int16 {
	t.Helper()
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	pcm := make([]int16, len(raw)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return pcm
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return b
}

func TestEncodeMatchesReference(t *testing.T) {
	in := readPCM(t, "testdata/input.pcm")
	want := readFile(t, "testdata/encoded.g722")
	got := make([]byte, len(in)/2)
	assert.Equal(t, len(want), NewEncoder().Encode(got, in))
	assert.Equal(t, want, got, "bit-exact with the C implementation")
}

func TestDecodeMatchesReference(t *testing.T) {
	in := readFile(t, "testdata/encoded.g722")
	want := readPCM(t, "testdata/decoded.pcm")
	got := make([]int16, 2*len(in))
	assert.Equal(t, len(want), NewDecoder().Decode(got, in))
	assert.Equal(t, want, got, "bit-exact with the C implementation")
}

// Processing 20 ms frames (320 samples, 160 bytes) as with RTP must give
// the same result as processing everything at once: state carries over
// between calls.
func TestFramesKeepState(t *testing.T) {
	in := readPCM(t, "testdata/input.pcm")
	enc, dec := NewEncoder(), NewDecoder()
	var bits []byte
	var out []int16
	frame := make([]byte, 160)
	pcm := make([]int16, 320)
	for i := 0; i+320 <= len(in); i += 320 {
		enc.Encode(frame, in[i:i+320])
		bits = append(bits, frame...)
		dec.Decode(pcm, frame)
		out = append(out, pcm...)
	}
	assert.Equal(t, readFile(t, "testdata/encoded.g722"), bits)
	assert.Equal(t, readPCM(t, "testdata/decoded.pcm"), out)
}

// At full scale the original decoder wrapped around: values near +32767
// came out near -32768, audible as clicks.
func TestFullScaleDoesNotWrap(t *testing.T) {
	for _, amp := range []float64{32767, -32768} {
		in := make([]int16, 16000)
		for i := range in {
			// 1 kHz at full scale, clipped like in a mixer.
			in[i] = int16(math.Max(-32768, math.Min(32767, math.Round(amp*math.Sin(2*math.Pi*1000*float64(i)/16000)))))
		}
		bits := make([]byte, len(in)/2)
		NewEncoder().Encode(bits, in)
		out := make([]int16, len(in))
		NewDecoder().Decode(out, bits)
		for i := 1; i < len(out); i++ {
			// The largest step of a 1 kHz sine is about 12800.
			step := int(out[i]) - int(out[i-1])
			require.InDelta(t, 0, step, 20000, "amplitude %v: jump at sample %d", amp, i)
		}
	}
}

// A sine must survive encoding and decoding with about the same level;
// this checks the translation independently of the original.
func TestRoundTripKeepsSine(t *testing.T) {
	for _, freq := range []float64{440, 1000, 3000, 6000} {
		const n = 16000
		in := make([]int16, n)
		for i := range in {
			in[i] = int16(math.Round(10000 * math.Sin(2*math.Pi*freq*float64(i)/16000)))
		}
		bits := make([]byte, n/2)
		NewEncoder().Encode(bits, in)
		out := make([]int16, n)
		NewDecoder().Decode(out, bits)

		// The filter banks delay the signal by 22 samples. After 0.1 s
		// (settled) the error is measured against the delayed input.
		const delay = 22
		var signal, noise float64
		for i := 1600; i < n; i++ {
			s := float64(in[i-delay])
			e := float64(out[i]) - s
			signal += s * s
			noise += e * e
		}
		snr := 10 * math.Log10(signal/noise)
		assert.Greater(t, snr, 20.0, "%v Hz: signal-to-noise ratio in dB", freq)
	}
}

func TestEncodeRejectsBadSizes(t *testing.T) {
	assert.PanicsWithValue(t, "g722: Encode needs an even number of samples", func() {
		NewEncoder().Encode(make([]byte, 2), make([]int16, 3)) //nolint:staticcheck // odd length on purpose
	})
	assert.PanicsWithValue(t, "g722: Encode destination too short", func() {
		NewEncoder().Encode(make([]byte, 1), make([]int16, 4))
	})
	assert.PanicsWithValue(t, "g722: Decode destination too short", func() {
		NewDecoder().Decode(make([]int16, 3), make([]byte, 2))
	})
}
