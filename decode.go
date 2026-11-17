package g722

// qm6 is the 6-bit inverse quantizer of the lower band.
var qm6 = [64]int{
	-136, -136, -136, -136,
	-24808, -21904, -19008, -16704,
	-14984, -13512, -12280, -11192,
	-10232, -9360, -8576, -7856,
	-7192, -6576, -6000, -5456,
	-4944, -4464, -4008, -3576,
	-3168, -2776, -2400, -2032,
	-1688, -1360, -1040, -728,
	24808, 21904, 19008, 16704,
	14984, 13512, 12280, 11192,
	10232, 9360, 8576, 7856,
	7192, 6576, 6000, 5456,
	4944, 4464, 4008, 3576,
	3168, 2776, 2400, 2032,
	1688, 1360, 1040, 728,
	432, 136, -432, -136,
}

// Decoder decodes one audio stream.
type Decoder struct {
	x    [24]int // signal history for the filter bank
	band [2]band
}

// NewDecoder returns a decoder in its initial state.
func NewDecoder() *Decoder {
	return &Decoder{band: newBands()}
}

// Decode decodes src into dst (16 kHz) and returns the number of samples
// written, 2*len(src). dst must be at least that long; otherwise Decode
// panics.
func (d *Decoder) Decode(dst []int16, src []byte) int {
	if len(dst) < 2*len(src) {
		panic("g722: Decode destination too short")
	}
	for k, code := range src {
		rlow := d.band[0].decodeLow(int(code) & 0x3F)
		rhigh := d.band[1].decodeHigh(int(code) >> 6)

		// Filter bank: joins both bands into two samples. The original
		// truncates to 16 bits here, which wraps around at full scale and
		// causes loud clicks; this version clamps instead.
		copy(d.x[:22], d.x[2:])
		d.x[22] = rlow + rhigh
		d.x[23] = rlow - rhigh
		xout1, xout2 := 0, 0
		for i := range 12 {
			xout2 += d.x[2*i] * qmfCoeffs[i]
			xout1 += d.x[2*i+1] * qmfCoeffs[11-i]
		}
		dst[2*k] = toInt16(xout1 >> 11)
		dst[2*k+1] = toInt16(xout2 >> 11)
	}
	return 2 * len(src)
}

// decodeLow decodes a 6-bit lower sub-band code and returns the
// reconstructed sample.
func (b *band) decodeLow(ilr int) int {
	// INVQBL, RECONS, LIMIT
	rlow := min(max(b.s+(b.det*qm6[ilr])>>15, -16384), 16383)

	// INVQAL, LOGSCL, SCALEL
	ril := ilr >> 2
	dlow := (b.det * qm4[ril]) >> 15
	b.nb = min(max((b.nb*127)>>7+wl[rl42[ril]], 0), 18432)
	b.det = scale(b.nb, 8)
	b.update(dlow)
	return rlow
}

// decodeHigh decodes a 2-bit upper sub-band code and returns the
// reconstructed sample.
func (b *band) decodeHigh(ih int) int {
	// INVQAH, RECONS, LIMIT
	dhigh := (b.det * qm2[ih]) >> 15
	rhigh := min(max(dhigh+b.s, -16384), 16383)

	// LOGSCH, SCALEH
	b.nb = min(max((b.nb*127)>>7+wh[rh2[ih]], 0), 22528)
	b.det = scale(b.nb, 10)
	b.update(dhigh)
	return rhigh
}
