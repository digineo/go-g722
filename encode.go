package g722

// Tables used only by the encoder.
var (
	q6 = [32]int{
		0, 35, 72, 110, 150, 190, 233, 276,
		323, 370, 422, 473, 530, 587, 650, 714,
		786, 858, 940, 1023, 1121, 1219, 1339, 1458,
		1612, 1765, 1980, 2195, 2557, 2919, 0, 0,
	}
	iln = [32]int{
		0, 63, 62, 31, 30, 29, 28, 27,
		26, 25, 24, 23, 22, 21, 20, 19,
		18, 17, 16, 15, 14, 13, 12, 11,
		10, 9, 8, 7, 6, 5, 4, 0,
	}
	ilp = [32]int{
		0, 61, 60, 59, 58, 57, 56, 55,
		54, 53, 52, 51, 50, 49, 48, 47,
		46, 45, 44, 43, 42, 41, 40, 39,
		38, 37, 36, 35, 34, 33, 32, 0,
	}
	ihn = [3]int{0, 1, 0}
	ihp = [3]int{0, 3, 2}
)

// Encoder encodes one audio stream.
type Encoder struct {
	x    [24]int // signal history for the filter bank
	band [2]band
}

// NewEncoder returns an encoder in its initial state.
func NewEncoder() *Encoder {
	return &Encoder{band: newBands()}
}

// Encode encodes src (16 kHz) into dst and returns the number of bytes
// written, len(src)/2. len(src) must be even and dst at least len(src)/2
// bytes long; otherwise Encode panics.
func (e *Encoder) Encode(dst []byte, src []int16) int {
	if len(src)%2 != 0 {
		panic("g722: Encode needs an even number of samples")
	}
	n := len(src) / 2
	if len(dst) < n {
		panic("g722: Encode destination too short")
	}
	dst = dst[:n]
	for k := range dst {
		// Filter bank: two samples yield one sample in each of the lower
		// and upper band.
		copy(e.x[:22], e.x[2:])
		e.x[22] = int(src[2*k])
		e.x[23] = int(src[2*k+1])
		sumEven, sumOdd := 0, 0
		for i := range 12 {
			sumOdd += e.x[2*i] * qmfCoeffs[i]
			sumEven += e.x[2*i+1] * qmfCoeffs[11-i]
		}
		ilow := e.band[0].encodeLow((sumEven + sumOdd) >> 14)
		ihigh := e.band[1].encodeHigh((sumEven - sumOdd) >> 14)
		dst[k] = byte(ihigh<<6 | ilow) //nolint:gosec // k ranges over dst; G602 false positive
	}
	return n
}

// encodeLow encodes one lower sub-band sample and returns the 6-bit code.
func (b *band) encodeLow(xlow int) int {
	// SUBTRA, QUANTL
	el := saturate(xlow - b.s)
	wd := el
	if el < 0 {
		wd = -(el + 1)
	}
	i := 1
	for ; i < 30; i++ {
		if wd < (q6[i]*b.det)>>12 {
			break
		}
	}
	ilow := ilp[i]
	if el < 0 {
		ilow = iln[i]
	}

	// INVQAL, LOGSCL, SCALEL
	ril := ilow >> 2
	dlow := (b.det * qm4[ril]) >> 15
	b.nb = min(max((b.nb*127)>>7+wl[rl42[ril]], 0), 18432)
	b.det = scale(b.nb, 8)
	b.update(dlow)
	return ilow
}

// encodeHigh encodes one upper sub-band sample and returns the 2-bit code.
func (b *band) encodeHigh(xhigh int) int {
	// SUBTRA, QUANTH
	eh := saturate(xhigh - b.s)
	wd := eh
	if eh < 0 {
		wd = -(eh + 1)
	}
	mih := 1
	if wd >= (564*b.det)>>12 {
		mih = 2
	}
	ihigh := ihp[mih]
	if eh < 0 {
		ihigh = ihn[mih]
	}

	// INVQAH, LOGSCH, SCALEH
	dhigh := (b.det * qm2[ihigh]) >> 15
	b.nb = min(max((b.nb*127)>>7+wh[rh2[ihigh]], 0), 22528)
	b.det = scale(b.nb, 10)
	b.update(dhigh)
	return ihigh
}
