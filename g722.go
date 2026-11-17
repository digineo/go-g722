// Package g722 encodes and decodes audio according to ITU-T G.722 at
// 64 kbit/s: 16 kHz, 16-bit linear PCM, mono. Every two samples become
// one byte.
//
// Encoders and decoders are stateful (sub-band ADPCM). Each audio stream
// needs its own, and they must not be used from several goroutines at
// the same time.
//
// The code is a translation of Steve Underwood's C implementation (via
// dgoncharov/g722tools, BSD-3-Clause), which is based on the 1993 G.722
// codec of Carnegie Mellon University. Unlike the original, the decoder
// clamps its output to the int16 range instead of wrapping around.
package g722

// band is the state of one sub-band (predictor and scale factor), the
// same for encoder and decoder.
type band struct {
	s, sp, sz int
	r, a, ap  [3]int
	p         [3]int
	d, b, bp  [7]int
	sg        [7]int
	nb, det   int
}

// newBands returns the initial state of both sub-bands.
func newBands() [2]band {
	var b [2]band
	b[0].det = 32
	b[1].det = 8
	return b
}

// qmfCoeffs are the coefficients of the quadrature mirror filter bank that
// splits the signal into the lower and upper sub-band and joins them again.
var qmfCoeffs = [12]int{3, -11, 12, 32, -210, 951, 3876, -805, 362, -156, 53, -11}

// Tables shared by encoder and decoder.
var (
	wl   = [8]int{-60, -30, 58, 172, 334, 538, 1198, 3042}
	rl42 = [16]int{0, 7, 6, 5, 4, 3, 2, 1, 7, 6, 5, 4, 3, 2, 1, 0}
	ilb  = [32]int{
		2048, 2093, 2139, 2186, 2233, 2282, 2332,
		2383, 2435, 2489, 2543, 2599, 2656, 2714,
		2774, 2834, 2896, 2960, 3025, 3091, 3158,
		3228, 3298, 3371, 3444, 3520, 3597, 3676,
		3756, 3838, 3922, 4008,
	}
	qm4 = [16]int{
		0, -20456, -12896, -8968,
		-6288, -4240, -2584, -1200,
		20456, 12896, 8968, 6288,
		4240, 2584, 1200, 0,
	}
	qm2 = [4]int{-7408, -1616, 7408, 1616}
	wh  = [3]int{0, -214, 798}
	rh2 = [4]int{2, 1, 2, 1}
)

// saturate clamps v to the int16 range.
func saturate(v int) int {
	return min(max(v, -32768), 32767)
}

// toInt16 clamps v to the int16 range and converts it.
func toInt16(v int) int16 {
	return int16(saturate(v)) //nolint:gosec // saturate keeps v in range
}

// scale computes the scale factor det from nb (blocks 3L and 3H); shift
// is 8 for the lower and 10 for the upper band.
func scale(nb, shift int) int {
	wd1 := (nb >> 6) & 31
	wd2 := shift - (nb >> 11)
	var wd3 int
	if wd2 < 0 {
		wd3 = ilb[wd1] << -wd2
	} else {
		wd3 = ilb[wd1] >> wd2
	}
	return wd3 << 2
}

// update adapts the predictor of a sub-band to the quantized difference
// signal d (block 4 of the standard).
func (b *band) update(d int) {
	// RECONS, PARREC
	b.d[0] = d
	b.r[0] = saturate(b.s + d)
	b.p[0] = saturate(b.sz + d)

	// UPPOL2
	for i := range 3 {
		b.sg[i] = b.p[i] >> 15
	}
	wd1 := saturate(b.a[1] << 2)
	wd2 := wd1
	if b.sg[0] == b.sg[1] {
		wd2 = -wd1
	}
	wd2 = min(wd2, 32767)
	wd3 := -128
	if b.sg[0] == b.sg[2] {
		wd3 = 128
	}
	wd3 += wd2 >> 7
	wd3 += (b.a[2] * 32512) >> 15
	b.ap[2] = min(max(wd3, -12288), 12288)

	// UPPOL1
	b.sg[0] = b.p[0] >> 15
	b.sg[1] = b.p[1] >> 15
	wd1 = -192
	if b.sg[0] == b.sg[1] {
		wd1 = 192
	}
	wd2 = (b.a[1] * 32640) >> 15
	b.ap[1] = saturate(wd1 + wd2)
	wd3 = saturate(15360 - b.ap[2])
	b.ap[1] = min(max(b.ap[1], -wd3), wd3)

	// UPZERO
	wd1 = 128
	if d == 0 {
		wd1 = 0
	}
	b.sg[0] = d >> 15
	for i := 1; i < 7; i++ {
		b.sg[i] = b.d[i] >> 15
		wd2 = -wd1
		if b.sg[i] == b.sg[0] {
			wd2 = wd1
		}
		wd3 = (b.b[i] * 32640) >> 15
		b.bp[i] = saturate(wd2 + wd3)
	}

	// DELAYA
	for i := 6; i > 0; i-- {
		b.d[i] = b.d[i-1]
		b.b[i] = b.bp[i]
	}
	for i := 2; i > 0; i-- {
		b.r[i] = b.r[i-1]
		b.p[i] = b.p[i-1]
		b.a[i] = b.ap[i]
	}

	// FILTEP
	wd1 = saturate(b.r[1] + b.r[1])
	wd1 = (b.a[1] * wd1) >> 15
	wd2 = saturate(b.r[2] + b.r[2])
	wd2 = (b.a[2] * wd2) >> 15
	b.sp = saturate(wd1 + wd2)

	// FILTEZ
	b.sz = 0
	for i := 6; i > 0; i-- {
		wd1 = saturate(b.d[i] + b.d[i])
		b.sz += (b.b[i] * wd1) >> 15
	}
	b.sz = saturate(b.sz)

	// PREDIC
	b.s = saturate(b.sp + b.sz)
}
